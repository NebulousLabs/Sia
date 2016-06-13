package contractor

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	errAllowanceNoHosts    = errors.New("hosts must be non-zero")
	errAllowanceZeroPeriod = errors.New("period must be non-zero")
	errAllowanceZeroWindow = errors.New("renew window must be non-zero")
	errAllowanceWindowSize = errors.New("renew window must be less than period")
)

// contractEndHeight returns the height at which the Contractor's contracts
// end. If there are no contracts, it returns zero.
func (c *Contractor) contractEndHeight() types.BlockHeight {
	var endHeight types.BlockHeight
	for _, contract := range c.contracts {
		endHeight = contract.EndHeight()
		break
	}
	// sanity check: all contracts should have same EndHeight
	if build.DEBUG {
		for _, contract := range c.contracts {
			if contract.EndHeight() != endHeight {
				build.Critical("all contracts should have EndHeight", endHeight, "-- got", contract.EndHeight())
			}
		}
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
		return errAllowanceZeroWindow
	} else if a.RenewWindow >= a.Period {
		return errAllowanceWindowSize
	}

	// check that allowance is sufficient to store at least one sector
	numSectors, err := maxSectors(a, c.hdb)
	if err != nil {
		return err
	} else if numSectors == 0 {
		return errInsufficientAllowance
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
	endHeight := c.blockHeight + a.Period
	if a.Period == c.allowance.Period && len(c.contracts) > 0 {
		endHeight = c.contractEndHeight()
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
