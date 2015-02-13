package transactionpool

import (
	"github.com/NebulousLabs/Sia/consensus"
)

func (tp *TransactionPool) update() {
	tp.state.RLock()
	defer tp.state.RUnlock()

	// Get the block diffs since the previous update.
	removedBlocksIDs, addedBlocksIDs, err := tp.state.BlocksSince(tp.recentBlock)
	if err != nil {
		if consensus.DEBUG {
			panic("BlocksSince returned an error?")
		}
		return
	}
	var removedBlocks, addedBlocks []consensus.Block
	for _, id := range removedBlocksIDs {
		block, exists := tp.state.Block(id)
		if !exists {
			if consensus.DEBUG {
				panic("state returned a block that doesn't exist?")
			}
			return
		}
		removedBlocks = append(removedBlocks, block)
	}
	for _, id := range addedBlocksIDs {
		block, exists := tp.state.Block(id)
		if !exists {
			if consensus.DEBUG {
				panic("state returned a block that doesn't exist?")
			}
			return
		}
		addedBlocks = append(addedBlocks, block)
	}

	// Add all of the removed transactions into the linked list.
	for _, block := range removedBlocks {
		for j := len(block.Transactions) - 1; j >= 0; j-- {
			txn := block.Transactions[j]

			ut := &unconfirmedTransaction{
				transaction: txn,
				dependents:  make(map[*unconfirmedTransaction]struct{}),
			}

			tp.applySiacoinInputs(txn, ut)
			tp.applySiacoinOutputs(txn, ut)
			tp.applyFileContracts(txn, ut)
			tp.applyFileContractTerminations(txn, ut)
			tp.applyStorageProofs(txn, ut)
			tp.applySiafundInputs(txn, ut)
			tp.applySiafundOutputs(txn, ut)

			// Add the transaction to the front of the linked list.
			tp.prependUnconfirmedTransaction(ut)

			// If the transaction contains a storage proof or is non-standard,
			// remove this transaction from the pool. This is done last because
			// we also need to remove any dependents.
			err = tp.IsStandardTransaction(txn)
			if err != nil {
				tp.removeUnconfirmedTransactionFromPool()
			}
		}
	}

	// Once moving forward again, remove any conflicts in the linked list that
	// occur with transactions that got accepted.
	for _, block := range addedBlocks {
		for _, txn := range block.Transactions {
			for _, input := range txn.SiacoinInputs {

				// TODO: Determine if there's a conflict.
				var conflict bool

				if conflict {
					tp.removeUnconfirmedTransactionFromPool(conflict)
				}
			}
		}
	}

	tp.recentBlock = tp.state.CurrentBlock().ID()
}
