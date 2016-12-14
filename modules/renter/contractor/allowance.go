package contractor

import (
	"errors"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	errAllowanceNoHosts    = errors.New("hosts must be non-zero")
	errAllowanceZeroPeriod = errors.New("period must be non-zero")
	errAllowanceWindowSize = errors.New("renew window must be less than period")

	// ErrAllowanceZeroWindow is returned when the caller requests a
	// zero-length renewal window. This will happen if the caller sets the
	// period to 1 block, since RenewWindow := period / 2.
	ErrAllowanceZeroWindow = errors.New("renew window must be non-zero")
)

// contractEndHeight returns the height at which the Contractor's contracts
// end. If there are no contracts, it returns zero.
func (c *Contractor) contractEndHeight() types.BlockHeight {
	var endHeight types.BlockHeight
	for _, contract := range c.contracts {
		endHeight = contract.EndHeight()
		break
	}
	return endHeight
}

// SetAllowance sets the amount of money the Contractor is allowed to spend on
// contracts over a given time period, divided among the number of hosts
// specified. Note that Contractor can start forming contracts as soon as
// SetAllowance is called; that is, it may block.
//
// In most cases, SetAllowance will renew existing contracts instead of
// forming new ones. This preserves the data on those hosts. When this occurs,
// the renewed contracts will atomically replace their previous versions. If
// SetAllowance is interrupted, renewed contracts may be lost, though the
// allocated funds will eventually be returned,
//
// TODO: can an Editor or Downloader be used across renewals?
// TODO: will hosts allow renewing the same contract twice?
//
// NOTE: At this time, transaction fees are not counted towards the allowance.
// This means the contractor may spend more than allowance.Funds.
func (c *Contractor) SetAllowance(a modules.Allowance) error {
	// sanity checks
	if a.Hosts == 0 {
		return errAllowanceNoHosts
	} else if a.Period == 0 {
		return errAllowanceZeroPeriod
	} else if a.RenewWindow == 0 {
		return ErrAllowanceZeroWindow
	} else if a.RenewWindow >= a.Period {
		return errAllowanceWindowSize
	}

	// calculate the maximum sectors this allowance can store
	max, err := maxSectors(a, c.hdb, c.tpool)
	if err != nil {
		return err
	}
	// Only allocate half as many sectors as the max. This leaves some leeway
	// for replacing contracts, transaction fees, etc.
	numSectors := max / 2
	// check that this is sufficient to store at least one sector
	if numSectors == 0 {
		return ErrInsufficientAllowance
	}

	c.mu.RLock()
	shouldRenew := a.Period != c.allowance.Period || a.Funds.Cmp(c.allowance.Funds) != 0
	shouldWait := c.blockHeight+a.Period < c.contractEndHeight()
	remaining := int(a.Hosts) - len(c.contracts)
	c.mu.RUnlock()

	if !shouldRenew {
		// If no contracts need renewing, just form new contracts.
		return c.managedFormAllowanceContracts(remaining, numSectors, a)
	} else if shouldWait {
		// If the new period would result in an earlier endHeight, we can't
		// renew; instead, set the allowance without modifying any contracts.
		// They will be renewed with the new allowance when they expire.
		c.mu.Lock()
		c.allowance = a
		c.periodStart = c.blockHeight
		err = c.saveSync()
		c.mu.Unlock()
		return err
	}

	c.mu.RLock()
	// gather contracts to renew
	var renewSet []modules.RenterContract
	for _, contract := range c.contracts {
		renewSet = append(renewSet, contract)
	}

	// calculate new endHeight; if the period has not changed, the endHeight
	// should not change either
	periodStart := c.blockHeight
	endHeight := periodStart + a.Period
	if a.Period == c.allowance.Period && len(c.contracts) > 0 {
		// COMPAT v0.6.0 - old hosts require end height increase by at least 1
		endHeight = c.contractEndHeight() + 1
	}
	c.mu.RUnlock()

	// renew existing contracts with new allowance parameters
	newContracts := make(map[types.FileContractID]modules.RenterContract)
	for _, contract := range renewSet {
		newContract, err := c.managedRenew(contract, numSectors, endHeight)
		if err != nil {
			c.log.Printf("WARN: failed to renew contract with %v; a new contract will be formed in its place", contract.NetAddress)
			remaining++
			continue
		}
		newContracts[newContract.ID] = newContract
		if len(newContracts) >= int(a.Hosts) {
			break
		}
		if build.Release != "testing" {
			// sleep for 1 minute to alleviate potential block propagation issues
			time.Sleep(60 * time.Second)
		}
	}

	// if we did not renew enough contracts, form new ones
	if remaining > 0 {
		formed, err := c.managedFormContracts(remaining, numSectors, endHeight)
		if err != nil {
			return err
		}
		for _, contract := range formed {
			newContracts[contract.ID] = contract
		}
	}

	// if we weren't able to form anything, return an error
	if len(newContracts) == 0 {
		return errors.New("unable to form or renew any contracts")
	}

	// Set the allowance and replace the contract set
	c.mu.Lock()
	c.allowance = a
	c.contracts = newContracts
	// update metrics
	var spending types.Currency
	for _, contract := range c.contracts {
		spending = spending.Add(contract.RenterFunds())
	}
	c.financialMetrics.ContractSpending = spending
	c.periodStart = periodStart
	err = c.saveSync()
	c.mu.Unlock()

	return err
}

// managedFormAllowanceContracts handles the special case where no contracts
// need to be renewed when setting the allowance.
func (c *Contractor) managedFormAllowanceContracts(n int, numSectors uint64, a modules.Allowance) error {
	if n <= 0 {
		return nil
	}

	// if we're forming contracts but not renewing, the new contracts should
	// have the same endHeight as the existing ones. Otherwise, the endHeight
	// should be a full period.
	c.mu.RLock()
	endHeight := c.blockHeight + a.Period
	if len(c.contracts) > 0 {
		endHeight = c.contractEndHeight()
	}
	c.mu.RUnlock()

	// form the contracts
	formed, err := c.managedFormContracts(n, numSectors, endHeight)
	if err != nil {
		return err
	}

	// Set the allowance and replace the contract set
	c.mu.Lock()
	c.allowance = a
	for _, contract := range formed {
		c.contracts[contract.ID] = contract
	}
	// update metrics
	var spending types.Currency
	for _, contract := range c.contracts {
		spending = spending.Add(contract.RenterFunds())
	}
	c.financialMetrics.ContractSpending = spending
	err = c.saveSync()
	c.mu.Unlock()

	return err
}
