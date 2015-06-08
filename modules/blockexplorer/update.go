package blockexplorer

import (
	"fmt"

	"github.com/NebulousLabs/Sia/modules"
)

// Handles updates recieved from the consensus subscription, currently
// just removing and inserting blocks from the blockchain
func (es *ExplorerState) ReceiveConsensusSetUpdate(cc modules.ConsensusChange) {
	// Do lock stuff here
	lockID := es.mu.Lock()
	defer es.mu.Unlock(lockID)

	// Remove old blocks from the back of the blockchain
	for _, oldblock := range cc.RevertedBlocks {
		// Confirm that the reverted block is the last in the
		// blockchain
		// TODO: ask about this assumption
		if oldblock.ID() != es.Blocks[len(es.Blocks)-1].ID() {
			panic("Block reverted is not at end of blockchain")
		}

		es.Blocks = es.Blocks[:len(es.Blocks)-1]
	}

	// Add new blocks to the end of the blockchain
	for _, newblock := range cc.AppliedBlocks {
		es.Blocks = append(es.Blocks, newblock)
	}
}
