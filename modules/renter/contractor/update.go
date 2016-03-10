package contractor

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// renewThreshold calculates an appropriate renewThreshold for a given period.
func renewThreshold(period types.BlockHeight) types.BlockHeight {
	threshold := period / 4
	if threshold > 1000 {
		threshold = 1000
	}
	return threshold
}

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

	// renew/replace contracts
	if c.blockHeight+renewThreshold(c.allowance.Period) >= c.renewHeight {
		c.renewHeight += c.allowance.Period
		go c.threadedRenewContracts(c.allowance, c.renewHeight)
	}
}
