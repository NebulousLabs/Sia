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
	errAllowanceNotSynced  = errors.New("you must be synced to set an allowance")

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
// allocated funds will eventually be returned.
//
// If a is the empty allowance, SetAllowance will archive the current contract
// set. The contracts cannot be used to create Editors or Downloads, and will
// not be renewed.
//
// TODO: can an Editor or Downloader be used across renewals?
// TODO: will hosts allow renewing the same contract twice?
//
// NOTE: At this time, transaction fees are not counted towards the allowance.
// This means the contractor may spend more than allowance.Funds.
func (c *Contractor) SetAllowance(a modules.Allowance) error {
	if a.Funds.IsZero() && a.Hosts == 0 && a.Period == 0 && a.RenewWindow == 0 {
		return c.managedCancelAllowance(a)
	}

	// sanity checks
	if a.Hosts == 0 {
		return errAllowanceNoHosts
	} else if a.Period == 0 {
		return errAllowanceZeroPeriod
	} else if a.RenewWindow == 0 {
		return ErrAllowanceZeroWindow
	} else if a.RenewWindow >= a.Period {
		return errAllowanceWindowSize
	} else if !c.cs.Synced() {
		return errAllowanceNotSynced
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

	c.log.Println("INFO: setting allowance to", a)

	c.mu.RLock()
	shouldRenew := a.Period != c.allowance.Period || !a.Funds.Equals(c.allowance.Funds)
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
	periodStart := c.blockHeight
	endHeight := periodStart + a.Period
	if a.Period == c.allowance.Period && len(c.contracts) > 0 {
		// COMPAT v0.6.0 - old hosts require end height increase by at least 1
		endHeight = c.contractEndHeight() + 1
	}
	c.mu.RUnlock()

	// renew existing contracts with new allowance parameters
	newContracts := make(map[types.FileContractID]modules.RenterContract)
	renewedIDs := make(map[types.FileContractID]types.FileContractID)
	for _, contract := range renewSet {
		newContract, err := c.managedRenew(contract, numSectors, endHeight)
		if err != nil {
			c.log.Printf("WARN: failed to renew contract with %v (error: %v); a new contract will be formed in its place", contract.NetAddress, err)
			remaining++
			continue
		}
		newContracts[newContract.ID] = newContract
		renewedIDs[contract.ID] = newContract.ID
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

	c.mu.Lock()
	// update the allowance
	c.allowance = a
	// archive the current contract set
	for id, contract := range c.contracts {
		c.oldContracts[id] = contract
	}
	// replace the current contract set with new contracts
	c.contracts = newContracts
	// link the contracts that were renewed
	for oldID, newID := range renewedIDs {
		c.renewedIDs[oldID] = newID
	}
	// if the currentPeriod was previously unset, set it now
	if c.currentPeriod == 0 {
		c.currentPeriod = periodStart
	}
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

	// Set the allowance and update the contract set
	c.mu.Lock()
	c.allowance = a
	for _, contract := range formed {
		c.contracts[contract.ID] = contract
	}
	err = c.saveSync()
	c.mu.Unlock()

	return err
}

// managedCancelAllowance handles the special case where the allowance is empty.
func (c *Contractor) managedCancelAllowance(a modules.Allowance) error {
	c.log.Println("INFO: canceling allowance")
	// first need to invalidate any active editors/downloaders
	// NOTE: this code is the same as in managedRenewContracts
	var ids []types.FileContractID
	c.mu.Lock()
	for id := range c.contracts {
		ids = append(ids, id)
		// we aren't renewing, but we don't want new editors or downloaders to
		// be created
		c.renewing[id] = true
	}
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		for _, id := range ids {
			delete(c.renewing, id)
		}
		c.mu.Unlock()
	}()
	for _, id := range ids {
		c.mu.RLock()
		e, eok := c.editors[id]
		d, dok := c.downloaders[id]
		c.mu.RUnlock()
		if eok {
			e.invalidate()
		}
		if dok {
			d.invalidate()
		}
	}

	// reset currentPeriod and archive all contracts
	c.mu.Lock()
	c.allowance = a
	c.currentPeriod = 0
	for id, contract := range c.contracts {
		c.oldContracts[id] = contract
	}
	c.contracts = make(map[types.FileContractID]modules.RenterContract)
	err := c.saveSync()
	c.mu.Unlock()
	return err
}
