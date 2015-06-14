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

	// Modify the number of file contracts and how much they costed
	for _, diff := range cc.FileContractDiffs {
		// Diff direction is a bool representing if the file
		// contract is new or getting removed. True signifies
		// that it is a new file contract
		if diff.Direction == true {
			be.activeContracts += 1
			be.totalContracts += 1
			be.activeContractCost = be.activeContractCost.Add(diff.FileContract.Payout)
			be.totalContractCost = be.totalContractCost.Add(diff.FileContract.Payout)
		} else {
			be.activeContracts -= 1
			be.activeContractCost = be.activeContractCost.Sub(diff.FileContract.Payout)
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
	}
	be.blockchainHeight += types.BlockHeight(len(cc.AppliedBlocks))
	be.currentBlock = cc.AppliedBlocks[len(cc.AppliedBlocks)-1]

	// Notify subscribers about updates
	be.updateSubscribers()
}
