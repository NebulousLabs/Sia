package blockexplorer

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// Handles updates recieved from the consensus subscription, currently
// just removing and inserting blocks from the blockchain
func (es *ExplorerState) ReceiveConsensusSetUpdate(cc modules.ConsensusChange) {
	// Do lock stuff here
	lockID := es.mu.Lock()
	defer es.mu.Unlock(lockID)

	// Update the current block, and block height
	es.blockchainHeight -= types.BlockHeight(len(cc.RevertedBlocks))
	es.blockchainHeight += types.BlockHeight(len(cc.AppliedBlocks))
	if len(cc.AppliedBlocks) > 0 {
		es.currentBlock = cc.AppliedBlocks[len(cc.AppliedBlocks)-1]
	}
}
