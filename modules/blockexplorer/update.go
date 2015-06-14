package blockexplorer

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// Handles updates recieved from the consensus subscription, currently
// just removing and inserting blocks from the blockchain
func (be *BlockExplorer) ReceiveConsensusSetUpdate(cc modules.ConsensusChange) {
	// Do lock stuff here
	lockID := be.mu.Lock()
	defer be.mu.Unlock(lockID)

	// Update the current block, and block height
	be.blockchainHeight -= types.BlockHeight(len(cc.RevertedBlocks))
	be.blockchainHeight += types.BlockHeight(len(cc.AppliedBlocks))
	if len(cc.AppliedBlocks) > 0 {
		be.currentBlock = cc.AppliedBlocks[len(cc.AppliedBlocks)-1]
	}

	// Notify subscribers about updates
	be.updateSubscribers()
}
