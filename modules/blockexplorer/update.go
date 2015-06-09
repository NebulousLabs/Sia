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
func (es *ExplorerState) ReceiveConsensusSetUpdate(cc modules.ConsensusChange) {
	lockID := es.mu.Lock()
	defer es.mu.Unlock(lockID)

	// Modify the currency value
	for _, diff := range cc.SiacoinOutputDiffs {
		if diff.Direction == true {
			es.currencySent = es.currencySent.Add(diff.SiacoinOutput.Value)
		}
	}

	// Modify the number of file contracts and their values
	for _, diff := range cc.FileContractDiffs {
		if diff.Direction == true {
			es.fileContracts += 1
			es.fileContractCost = es.fileContractCost.Add(diff.FileContract.Payout)
		} else {
			es.fileContracts -= 1
			es.fileContractCost = es.fileContractCost.Sub(diff.FileContract.Payout)
		}
	}

	// Reverting all the data from reverted blocks
	es.blockchainHeight -= types.BlockHeight(len(cc.RevertedBlocks))
	es.blocks = es.blocks[:len(es.blocks)-len(cc.RevertedBlocks)]

	// Handle incoming blocks
	for _, block := range cc.AppliedBlocks {

		// Must do some error checking in consensus
		blocktarget, exists := es.cs.ChildTarget(block.ID())
		if build.DEBUG {
			if !exists {
				panic("Applied block not in consensus")
			}
		}

		es.blocks = append(es.blocks, modules.BlockData{
			Timestamp: block.Timestamp,
			Target:    blocktarget,
			Size:      uint64(len(encoding.Marshal(block))),
		})
	}
	es.blockchainHeight += types.BlockHeight(len(cc.AppliedBlocks))
	es.currentBlock = cc.AppliedBlocks[len(cc.AppliedBlocks)-1]

	// Notify subscribers about updates
	be.updateSubscribers()
}
