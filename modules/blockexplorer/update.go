package blockexplorer

import (
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

	// Reverting all the data from reverted blocks
	for _, block := range cc.RevertedBlocks {
		for _, transaction := range block.Transactions {
			for _, output := range transaction.SiacoinOutputs {
				es.currencySent = es.currencySent.Sub(output.Value)
			}

			for _, contract := range transaction.FileContracts {
				es.currencySpent = es.currencySpent.Sub(contract.Payout)
			}
		}
	}
	es.blockchainHeight -= types.BlockHeight(len(cc.RevertedBlocks))
	es.timestamps = es.timestamps[:len(es.timestamps)-len(cc.RevertedBlocks)]
	es.blockSizes = es.blockSizes[:len(es.blockSizes)-len(cc.RevertedBlocks)]

	// Handle incoming blocks
	for _, block := range cc.AppliedBlocks {
		es.timestamps = append(es.timestamps, block.Timestamp)

		// Marshall the block to get the size of it
		es.blockSizes = append(es.blockSizes, uint64(len(encoding.Marshal(block))))

		// Add transaction data
		for _, transaction := range block.Transactions {
			for _, output := range transaction.SiacoinOutputs {
				es.currencySent = es.currencySent.Add(output.Value)
			}

			for _, contract := range transaction.FileContracts {
				es.currencySpent = es.currencySpent.Add(contract.Payout)
			}
		}
	}
	es.blockchainHeight += types.BlockHeight(len(cc.AppliedBlocks))
	es.currentBlock = cc.AppliedBlocks[len(cc.AppliedBlocks)-1]

	// Notify subscribers about updates
	be.updateSubscribers()
}
