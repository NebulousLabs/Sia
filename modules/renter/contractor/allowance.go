package contractor

import (
	"errors"
	"reflect"

	"github.com/NebulousLabs/Sia/modules"
)

var (
	errAllowanceNoHosts    = errors.New("hosts must be non-zero")
	errAllowanceNotSynced  = errors.New("you must be synced to set an allowance")
	errAllowanceWindowSize = errors.New("renew window must be less than period")
	errAllowanceZeroPeriod = errors.New("period must be non-zero")

	// ErrAllowanceZeroWindow is returned when the caller requests a
	// zero-length renewal window. This will happen if the caller sets the
	// period to 1 block, since RenewWindow := period / 2.
	ErrAllowanceZeroWindow = errors.New("renew window must be non-zero")
)

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
	if reflect.DeepEqual(a, modules.Allowance{}) {
		return c.managedCancelAllowance()
	}
	if reflect.DeepEqual(a, c.allowance) {
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

	c.log.Println("INFO: setting allowance to", a)
	c.mu.Lock()
	// set the current period to the blockheight if the existing allowance is
	// empty
	if reflect.DeepEqual(c.allowance, modules.Allowance{}) {
		c.currentPeriod = c.blockHeight
	}
	c.allowance = a
	err := c.saveSync()
	c.mu.Unlock()
	if err != nil {
		c.log.Println("Unable to save contractor after setting allowance:", err)
	}

	// Cycle through all contracts and unlock them again since they might have
	// been locked by managedCancelAllowance previously.
	ids := c.staticContracts.IDs()
	for _, id := range ids {
		contract, exists := c.staticContracts.Acquire(id)
		if !exists {
			continue
		}
		utility := contract.Utility()
		utility.Locked = false
		err := contract.UpdateUtility(utility)
		c.staticContracts.Return(contract)
		if err != nil {
			return err
		}
	}

	// Interrupt any existing maintenance and launch a new round of
	// maintenance.
	c.managedInterruptContractMaintenance()
	go c.threadedContractMaintenance()
	return nil
}

// managedCancelAllowance handles the special case where the allowance is empty.
func (c *Contractor) managedCancelAllowance() error {
	c.log.Println("INFO: canceling allowance")
	// first need to invalidate any active editors
	// NOTE: this code is the same as in managedRenewContracts
	ids := c.staticContracts.IDs()
	c.mu.Lock()
	for _, id := range ids {
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
		c.mu.RUnlock()
		if eok {
			e.invalidate()
		}
	}

	// Clear out the allowance and save.
	c.mu.Lock()
	c.allowance = modules.Allowance{}
	c.currentPeriod = 0
	err := c.saveSync()
	c.mu.Unlock()
	if err != nil {
		return err
	}

	// Issue an interrupt to any in-progress contract maintenance thread.
	c.managedInterruptContractMaintenance()

	// Cycle through all contracts and mark them as !goodForRenew and !goodForUpload
	ids = c.staticContracts.IDs()
	for _, id := range ids {
		contract, exists := c.staticContracts.Acquire(id)
		if !exists {
			continue
		}
		utility := contract.Utility()
		utility.GoodForRenew = false
		utility.GoodForUpload = false
		utility.Locked = true
		err := contract.UpdateUtility(utility)
		c.staticContracts.Return(contract)
		if err != nil {
			return err
		}
	}
	return nil
}
