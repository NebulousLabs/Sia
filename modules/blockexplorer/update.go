package blockexplorer

import (
	"fmt"

	"github.com/NebulousLabs/Sia/modules"
)

// Handles updates recieved from the consensus subscription, currently
// just removing and inserting blocks from the blockchain
func (bc *ExplorerBlockchain) ReceiveConsensusSetUpdate(cc modules.ConsensusChange) {
	// Do lock stuff here
	// lockID := bc.mu.Lock()
	// defer bc.mu.Unlock(lockID)

	fmt.Println("Consensus Updated")

}
