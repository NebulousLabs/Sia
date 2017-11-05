package contractor

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// managedArchiveContracts will figure out which contracts are no longer needed
// and move them to the historic set of contracts.
//
// TODO: This function should be performed by threadedContractMaintenance.
// threadedContractMaintenance will currently quit if there are no hosts, but it
// should at least run this code before quitting.
func (c *Contractor) managedArchiveContracts() {
	err := c.tg.Add()
	if err != nil {
		return
	}
	defer c.tg.Done()

	// Determine the current block height.
	c.mu.RLock()
	currentHeight := c.blockHeight
	c.mu.RUnlock()

	// Loop through the current set of contracts and migrate any expired ones to
	// the set of old contracts.
	for _, contract := range c.contracts.ViewAll() {
		if currentHeight > contract.EndHeight() {
			id := contract.ID
			c.mu.Lock()
			c.oldContracts[id] = contract
			c.mu.Unlock()
			if contract, ok := c.contracts.Acquire(id); ok {
				c.contracts.Delete(contract)
			}
			c.log.Println("INFO: archived expired contract", id)
		}
	}

	// Save.
	//
	// TODO: There's potentially an issue here in the future where the
	// contractSet and the set of oldContracts could desync if there's a power
	// outage between calling Delete and adding the contract to oldContracts. To
	// prevent that potential inconsistency, when the contract persistence gets
	// migreated to the contractSet, we'll need some strategy for guaranteeing
	// that the set of oldContracts stays consistent.
	c.mu.Lock()
	c.save()
	c.mu.Unlock()
}

// ProcessConsensusChange will be called by the consensus set every time there
// is a change in the blockchain. Updates will always be called in order.
func (c *Contractor) ProcessConsensusChange(cc modules.ConsensusChange) {
	c.mu.Lock()
	for _, block := range cc.RevertedBlocks {
		if block.ID() != types.GenesisID {
			c.blockHeight--
		}
	}
	for _, block := range cc.AppliedBlocks {
		if block.ID() != types.GenesisID {
			c.blockHeight++
		}
	}

	// If we have entered the next period, update currentPeriod
	// NOTE: "period" refers to the duration of contracts, whereas "cycle"
	// refers to how frequently the period metrics are reset.
	// TODO: How to make this more explicit.
	cycleLen := c.allowance.Period - c.allowance.RenewWindow
	if c.blockHeight >= c.currentPeriod+cycleLen {
		c.currentPeriod += cycleLen
		// COMPATv1.0.4-lts
		// if we were storing a special metrics contract, it will be invalid
		// after we enter the next period.
		delete(c.oldContracts, metricsContractID)
	}

	c.lastChange = cc.ID
	err := c.save()
	if err != nil {
		c.log.Println("Unable to save while processing a consensus change:", err)
	}
	c.mu.Unlock()

	// Perform contract maintenance if our blockchain is synced. Use a separate
	// goroutine so that the rest of the contractor is not blocked during
	// maintenance.
	if cc.Synced {
		go c.threadedContractMaintenance()
		go c.managedArchiveContracts()
	}
}
