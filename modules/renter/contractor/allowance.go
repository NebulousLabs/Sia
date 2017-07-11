package contractor

import (
	"errors"

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
	c.mu.Lock()
	c.allowance = a
	err = c.saveSync()
	c.mu.Unlock()
	if err != nil {
		c.log.Println("Unable to save contractor after setting allowance:", err)
	}

	// Initiate maintenance on the contracts, and then return.
	go c.threadedContractMaintenance()
	return nil
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

// FormDownloadOnlyContract form download only contract to share file
func (c *Contractor) FormDownloadOnlyContract(host modules.HostDBEntry) (modules.RenterContract, error) {
	a := c.allowance
	c.mu.RLock()
	var endHeight types.BlockHeight
	if a.Period > 0 {
		endHeight = c.blockHeight + a.Period
	} else {
		endHeight = c.blockHeight + 1008 // 6x24x7 which should be a week
	}
	if len(c.contracts) > 0 {
		endHeight = c.contractEndHeight()
	}
	c.mu.RUnlock()

	contract, err := c.managedNewContract(host, 1, endHeight)
	if err != nil {
		return contract, err
	}

	c.mu.Lock()
	// set GoodForUpload false to prevent upload to it accidentally
	contract.GoodForUpload = false
	c.contracts[contract.ID] = contract
	err = c.saveSync()
	c.mu.Unlock()

	return contract, err
}
