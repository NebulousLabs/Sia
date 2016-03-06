package contractor

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// ProcessConsensusChange will be called by the consensus set every time there
// is a change in the blockchain. Updates will always be called in order.
func (c *Contractor) ProcessConsensusChange(cc modules.ConsensusChange) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, block := range cc.RevertedBlocks {
		if block.ID() != types.GenesisBlock.ID() {
			c.blockHeight--
		}
	}
	for _, block := range cc.AppliedBlocks {
		if block.ID() != types.GenesisBlock.ID() {
			c.blockHeight++
		}
	}

	// renew contracts
	if c.blockHeight+c.allowance.RenewWindow >= c.renewHeight {
		c.renewHeight += c.allowance.Period
		go c.threadedRenewContracts(c.allowance, c.renewHeight)
		// reset the spentPeriod metric
		c.spentPeriod = types.ZeroCurrency
	}

	c.lastChange = cc.ID
}
