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

	if c.blockHeight != 0 || cc.AppliedBlocks[len(cc.AppliedBlocks)-1].ID() != types.GenesisBlock.ID() {
		c.blockHeight += types.BlockHeight(len(cc.AppliedBlocks))
		c.blockHeight -= types.BlockHeight(len(cc.RevertedBlocks))
	}

	// renew contracts
	if c.blockHeight+renewThreshold(c.allowance.Period) < c.renewHeight {
		newHeight := c.renewHeight + c.allowance.Period
		for _, contract := range c.contracts {
			if contract.FileContract.WindowStart == c.renewHeight {
				go c.threadedRenew(contract.ID, newHeight)
			}
		}
		c.renewHeight = newHeight
	}
}
