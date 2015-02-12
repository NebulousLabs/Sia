package transactionpool

import (
	"github.com/NebulousLabs/Sia/consensus"
)

func (tp *TransactionPool) removeTransactionFromPool(ut *unconfirmedTransaction) {
	// Remove this transaction from the linked list.
	tp.removeTransactionFromList(ut)

	// Remove self as a dependent from any requirements.
	for requirement := range ut.requirements {
		delete(requirement.dependents, ut)
	}

	// Remove each dependent from the transaction pool.
	for dependent := range ut.dependents {
		tp.removeTransactionFromPool(dependent)
	}
}

func (tp *TransactionPool) update() {
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
				transaction:  txn,
				requirements: make(map[*unconfirmedTransaction]struct{}),
				dependents:   make(map[*unconfirmedTransaction]struct{}),
			}

			// Find any transactions in our set that are dependent on this
			// transaction.
			for i := range txn.SiacoinOutputs {
				dependent, exists := tp.usedOutputs[txn.SiacoinOutputID(i)]
				if exists {
					ut.dependents[dependent] = struct{}{}
					dependent.requirements[ut] = struct{}{}
				}
			}

			// Add the transaction to the linked list.
			tp.addTransactionToHead(ut)

			// If the transaction contains a storage proof or is non-standard,
			// remove this transaction from the pool. This is done last because
			// we also need to remove any dependents.
			err = tp.IsStandardTransaction(txn)
			if err != nil || len(txn.StorageProofs) != 0 {
				// TODO: Call the function to remove a tranasction and
				// dependents.
			}
		}
	}

	// Once moving forward again, remove any conflicts in the linked list that
	// occur with transactions that got accepted.
	for _, block := range addedBlocks {
		for _, txn := range block.Transactions {
			for _, input := range txn.SiacoinInputs {
				conflict, exists := tp.usedOutputs[input.ParentID]
				if exists {
					tp.removeTransactionFromPool(conflict)
				}
			}
		}
	}

	tp.recentBlock = tp.state.CurrentBlock().ID()
}
