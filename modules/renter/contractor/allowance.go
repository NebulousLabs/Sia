package contractor

import (
	"errors"

	"github.com/NebulousLabs/Sia/modules"
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

// managedCancelAllowance handles the special case where the allowance is empty.
func (c *Contractor) managedCancelAllowance(a modules.Allowance) {
	c.log.Println("INFO: canceling allowance")

	// Update the allowance to specify zero hosts, and grab all existing hosts
	// toe delete them.
	c.mu.Lock()
	c.allowance.Hosts = 0
	contracts := make([]modules.RenterContract, 0, len(c.contracts))
	for _, contract := range c.contracts {
		contracts = append(contracts, contract)
	}
	c.mu.Unlock()

	// Cycle through all of the contracts and delete them.
	for _, contract := range contracts {
		c.managedLockContract(contract.ID)
		c.mu.Lock()
		// Archive the contract.
		c.oldContracts[contract.ID] = contract
		// Update the funds.
		c.allowance.Funds = c.allowance.Funds.Sub(contract.TotalCost)
		// Delete the contract.
		delete(c.contracts, contract.ID)
		// Save.
		err := c.saveSync()
		if err != nil {
			c.log.Println("Unable to save the contractor when deleting a contract:", err)
		}
		c.mu.Unlock()
		c.managedUnlockContract(contract.ID)
	}

	// reset currentPeriod and archive all contracts
	c.mu.Lock()
	c.allowance = a
	c.currentPeriod = 0
	c.mu.Unlock()
	return
}

// SetAllowance sets the amount of money the Contractor is allowed to spend on
// contracts over a given time period, divided among the number of hosts
// specified. Note that Contractor can start forming contracts as soon as
// SetAllowance is called; that is, it may block.
//
// If a is the empty allowance, SetAllowance will archive the current contract
// set. The contracts cannot be used to create Editors or Downloads, and will
// not be renewed.
func (c *Contractor) SetAllowance(a modules.Allowance) error {
	if a.Funds.IsZero() && a.Hosts == 0 && a.Period == 0 && a.RenewWindow == 0 {
		c.managedCancelAllowance(a)
		return nil
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

	// Update the allowance and spin off a round of contract repair.
	c.log.Println("INFO: setting allowance to", a)
	c.mu.Lock()
	a.Funds = c.allowance.Funds // Do not update the funds value.
	c.allowance = a
	c.mu.Unlock()
	go c.threadedRepairContracts()
	return nil
}
