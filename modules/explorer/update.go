package explorer

import (
	"fmt"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// ProcessConsensusChange follows the most recent changes to the consensus set,
// including parsing new blocks and updating the utxo sets.
func (e *Explorer) ProcessConsensusChange(cc modules.ConsensusChange) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Modify the number of file contracts and how much they costed
	for _, diff := range cc.FileContractDiffs {
		if diff.Direction == modules.DiffApply {
			e.activeContractCount += 1
			e.totalContractCount += 1
			e.activeContractCost = e.activeContractCost.Add(diff.FileContract.Payout)
			e.totalContractCost = e.totalContractCost.Add(diff.FileContract.Payout)
			e.activeContractSize = e.activeContractSize.Add(types.NewCurrency64(diff.FileContract.FileSize))
			e.totalContractSize = e.totalContractSize.Add(types.NewCurrency64(diff.FileContract.FileSize))
		} else {
			e.activeContractCount -= 1
			e.activeContractCost = e.activeContractCost.Sub(diff.FileContract.Payout)
			e.activeContractSize = e.activeContractSize.Sub(types.NewCurrency64(diff.FileContract.FileSize))
		}
	}

	// Reverting the blockheight and block data structs from reverted blocks
	e.blockchainHeight -= types.BlockHeight(len(cc.RevertedBlocks))

	// Handle incoming blocks
	for _, block := range cc.AppliedBlocks {
		e.blockchainHeight += 1

		// Add the current time to seenTimes
		if time.Unix(int64(block.Timestamp), 0).Before(e.startTime) {
			e.seenTimes[e.blockchainHeight%types.BlockHeight(len(e.seenTimes))] = time.Unix(int64(block.Timestamp), 0)
		} else {
			e.seenTimes[e.blockchainHeight%types.BlockHeight(len(e.seenTimes))] = time.Now()
		}

		// add the block to the database.
		err := e.addBlockDB(block)
		if err != nil {
			fmt.Printf("Error when adding block to database: " + err.Error() + "\n")
		}
	}
	e.currentBlock = cc.AppliedBlocks[len(cc.AppliedBlocks)-1]
}
