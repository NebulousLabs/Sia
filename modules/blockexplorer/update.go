package blockexplorer

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// Handles updates recieved from the consensus subscription. Keeps
// track of transaction volume, block timestamps and block sizes, as
// well as the current block height
func (be *BlockExplorer) ReceiveConsensusSetUpdate(cc modules.ConsensusChange) {
	lockID := be.mu.Lock()
	defer be.mu.Unlock(lockID)

	// Modify the currency value
	for _, diff := range cc.SiacoinOutputDiffs {
		if diff.Direction == true {
			be.currencySent = be.currencySent.Add(diff.SiacoinOutput.Value)
		}
	}

	// Modify the number of file contracts and their values
	for _, diff := range cc.FileContractDiffs {
		if diff.Direction == true {
			be.fileContracts += 1
			be.fileContractCost = be.fileContractCost.Add(diff.FileContract.Payout)
		} else {
			be.fileContracts -= 1
			be.fileContractCost = be.fileContractCost.Sub(diff.FileContract.Payout)
		}
	}

	// Reverting the blockheight and block data structs from reverted blocks
	be.blockchainHeight -= types.BlockHeight(len(cc.RevertedBlocks))
	be.blocks = be.blocks[:len(be.blocks)-len(cc.RevertedBlocks)]

	// Handle incoming blocks
	for _, block := range cc.AppliedBlocks {
		// Highly unlikely that consensus wouldn't have info
		// on a block it just published
		blocktarget, exists := be.cs.ChildTarget(block.ParentID)
		if build.DEBUG {
			if !exists {
				panic("Applied nblock not in consensus")
			}
		}

		// Marshall is used to get an exact byte size of the block
		be.blocks = append(be.blocks, modules.ExplorerBlockData{
			Timestamp: block.Timestamp,
			Target:    blocktarget,
			Size:      uint64(len(encoding.Marshal(block))),
		})
	}
	be.blockchainHeight += types.BlockHeight(len(cc.AppliedBlocks))
	be.currentBlock = cc.AppliedBlocks[len(cc.AppliedBlocks)-1]

	// Notify subscribers about updates
	be.updateSubscribers()
}
