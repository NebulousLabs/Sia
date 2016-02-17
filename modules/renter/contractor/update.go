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

	if c.blockHeight != 0 || cc.AppliedBlocks[len(cc.AppliedBlocks)-1].ID() != types.GenesisBlock.ID() {
		c.blockHeight += types.BlockHeight(len(cc.AppliedBlocks))
		c.blockHeight -= types.BlockHeight(len(cc.RevertedBlocks))
	}

	// Renew contracts here...
}
