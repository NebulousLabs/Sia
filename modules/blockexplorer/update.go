package blockexplorer

import (
	"fmt"

	"github.com/NebulousLabs/Sia/modules"
)

// Handles updates recieved from the consensus subscription, currently
// just removing and inserting blocks from the blockchain
func (bc *ExplorerBlockchain) ReceiveConsensusSetUpdate(cc modules.ConsensusChange) {
	// Do lock stuff here
	lockID := bc.mu.Lock()
	defer bc.mu.Unlock(lockID)

	// Remove old blocks from the back of the blockchain
	for _, oldblock := range cc.RevertedBlocks {
		// Confirm that the reverted block is the last in the
		// blockchain
		// TODO: ask about this assumption
		if oldblock.ID() != bc.Blocks[len(bc.Blocks)-1].ID() {
			panic("Block reverted is not at end of blockchain")
		}

		bc.Blocks = bc.Blocks[:len(bc.Blocks)-1]
	}

	// Add new blocks to the end of the blockchain
	for _, newblock := range cc.AppliedBlocks {
		bc.Blocks = append(bc.Blocks, newblock)
	}

	// Debug output
	fmt.Println("Blockchain changed:")
	for i, block := range bc.Blocks {
		fmt.Printf("Block %d: %x\n", i, block.ID())
	}

}
