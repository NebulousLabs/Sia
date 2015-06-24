package blockexplorer

import (
	"fmt"

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

	// Modify the number of file contracts and how much they costed
	for _, diff := range cc.FileContractDiffs {
		if diff.Direction == modules.DiffApply {
			be.activeContracts += 1
			be.totalContracts += 1
			be.activeContractCost = be.activeContractCost.Add(diff.FileContract.Payout)
			be.totalContractCost = be.totalContractCost.Add(diff.FileContract.Payout)
			be.activeContractSize += diff.FileContract.FileSize
			be.totalContractSize += diff.FileContract.FileSize
		} else {
			be.activeContracts -= 1
			be.activeContractCost = be.activeContractCost.Sub(diff.FileContract.Payout)
			be.activeContractSize -= diff.FileContract.FileSize
		}
	}

	// Reverting the blockheight and block data structs from reverted blocks
	be.blockchainHeight -= types.BlockHeight(len(cc.RevertedBlocks))
	be.blockSummaries = be.blockSummaries[:len(be.blockSummaries)-len(cc.RevertedBlocks)]

	// Handle incoming blocks
	for _, block := range cc.AppliedBlocks {
		// Special case for the genesis block, as it does not
		// have a valid parent id.
		var blocktarget types.Target
		if block.ID() == be.genesisBlockID {
			blocktarget = types.RootDepth
		} else {
			var exists bool
			blocktarget, exists = be.cs.ChildTarget(block.ParentID)
			if build.DEBUG {
				if !exists {
					panic("Applied block not in consensus")
				}
			}
		}

		// Marshall is used to get an exact byte size of the block
		be.blockSummaries = append(be.blockSummaries, modules.ExplorerBlockData{
			Timestamp: block.Timestamp,
			Target:    blocktarget,
			Size:      uint64(len(encoding.Marshal(block))),
		})

		err := be.addBlock(block)
		if err != nil {
			fmt.Printf("Error when adding block to database: " + err.Error() + "\n")
		}
		be.blockchainHeight += 1
	}
	be.currentBlock = cc.AppliedBlocks[len(cc.AppliedBlocks)-1]

	// Notify subscribers about updates
	be.updateSubscribers()
}
