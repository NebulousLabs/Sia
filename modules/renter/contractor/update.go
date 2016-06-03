package contractor

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

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

	// delete expired contracts
	var expired []types.FileContractID
	for id, contract := range c.contracts {
		// TODO: offset this by some sort of confirmation height?
		if c.blockHeight > contract.EndHeight() {
			expired = append(expired, id)
		}
	}
	for _, id := range expired {
		delete(c.contracts, id)
		c.log.Debugln("INFO: deleted expired contract", id)
	}

	c.lastChange = cc.ID
	err := c.save()
	if err != nil {
		c.log.Println(err)
	}
	c.mu.Unlock()

	// only attempt renewal if we are synced
	// (harmless otherwise, since hosts will reject our renewal attempts, but very slow)
	if c.cs.Synced() {
		// prevent allowance from being set until we have finished renewing
		c.contractLock.Lock()
		c.managedRenewContracts()
		c.contractLock.Unlock()
	}
}
