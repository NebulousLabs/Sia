package transactionpool

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
)

// removeUnconfirmedTransaction takes an unconfirmed transaction and removes it
// from the transaction pool, but leaves behind all dependencies.
func (tp *TransactionPool) removeUnconfirmedTransaction(ut *unconfirmedTransaction) {
	t := ut.transaction
	for _, sci := range t.SiacoinInputs {
		delete(tp.usedSiacoinOutputs, sci.ParentID)
	}
	for i, _ := range t.SiacoinOutputs {
		scoid := t.SiacoinOutputID(i)
		delete(tp.siacoinOutputs, scoid)
	}
	for i, fc := range t.FileContracts {
		fcid := t.FileContractID(i)
		delete(tp.fileContracts, fcid)
		delete(tp.newFileContracts[fc.Start], fcid)
	}
	for _, fct := range t.FileContractTerminations {
		delete(tp.fileContractTerminations, fct.ParentID)
	}
	for _, sp := range t.StorageProofs {
		fc, _ := tp.state.FileContract(sp.ParentID)
		triggerBlock, _ := tp.state.BlockAtHeight(fc.Start - 1)
		delete(tp.storageProofs[triggerBlock.ID()], sp.ParentID)
	}
	for _, sfi := range t.SiafundInputs {
		delete(tp.usedSiafundOutputs, sfi.ParentID)
	}
	for i, _ := range t.SiafundOutputs {
		sfoid := t.SiafundOutputID(i)
		delete(tp.siafundOutputs, sfoid)
	}
	delete(tp.transactions, crypto.HashObject(t))
	tp.removeUnconfirmedTransactionFromList(ut)
}

// removeDependentTransactions removes all unconfirmed transactions that are
// dependent on the input transaction.
func (tp *TransactionPool) removeDependentTransactions(t consensus.Transaction) {
	for i, _ := range t.SiacoinOutputs {
		dependent, exists := tp.usedSiacoinOutputs[t.SiacoinOutputID(i)]
		if exists {
			tp.purgeUnconfirmedTransaction(dependent)
		}
	}
	for i, fc := range t.FileContracts {
		dependent, exists := tp.fileContractTerminations[t.FileContractID(i)]
		if exists {
			tp.purgeUnconfirmedTransaction(dependent)
		}
		triggerBlock, _ := tp.state.BlockAtHeight(fc.Start - 1)
		dependent, exists = tp.storageProofs[triggerBlock.ID()][t.FileContractID(i)]
		if exists {
			tp.purgeUnconfirmedTransaction(dependent)
		}
	}
	for i, _ := range t.SiafundOutputs {
		dependent, exists := tp.usedSiafundOutputs[t.SiafundOutputID(i)]
		if exists {
			tp.purgeUnconfirmedTransaction(dependent)
		}
	}
}

// purgeUnconfirmedTransaction removes all transactions dependent on the input
// transaction, and then removes the input transaction.
func (tp *TransactionPool) purgeUnconfirmedTransaction(ut *unconfirmedTransaction) {
	t := ut.transaction
	tp.removeDependentTransactions(t)
	tp.removeUnconfirmedTransaction(ut)
}

// removeConflictingTransactions removes all of the transactions that are in
// conflict with the input transaction.
func (tp *TransactionPool) removeConflictingTransactions(t consensus.Transaction) {
	for _, sci := range t.SiacoinInputs {
		conflict, exists := tp.usedSiacoinOutputs[sci.ParentID]
		if exists {
			tp.purgeUnconfirmedTransaction(conflict)
		}
	}
	for _, fct := range t.FileContractTerminations {
		conflict, exists := tp.fileContractTerminations[fct.ParentID]
		if exists {
			tp.purgeUnconfirmedTransaction(conflict)
		}
		fc, _ := tp.state.FileContract(fct.ParentID)
		triggerBlock, _ := tp.state.BlockAtHeight(fc.Start - 1)
		conflict, exists = tp.storageProofs[triggerBlock.ID()][fct.ParentID]
		if exists {
			tp.purgeUnconfirmedTransaction(conflict)
		}
	}
	for _, sp := range t.StorageProofs {
		conflict, exists := tp.fileContractTerminations[sp.ParentID]
		if exists {
			tp.purgeUnconfirmedTransaction(conflict)
		}
		fc, _ := tp.state.FileContract(sp.ParentID)
		triggerBlock, _ := tp.state.BlockAtHeight(fc.Start - 1)
		conflict, exists = tp.storageProofs[triggerBlock.ID()][sp.ParentID]
		if exists {
			tp.purgeUnconfirmedTransaction(conflict)
		}
	}
	for _, sfi := range t.SiafundInputs {
		conflict, exists := tp.usedSiafundOutputs[sfi.ParentID]
		if exists {
			tp.purgeUnconfirmedTransaction(conflict)
		}
	}
}

// update grabs the recent set of block diffs from the state - the rewound
// blocks and the applied blocks, and updates the transaction pool by adding
// any transactions that got removed from the blockchain, and removing any
// transactions that are in conflict with new transactions, and also removing
// any transactions that have entered the blockchain.
func (tp *TransactionPool) update() {
	counter := tp.state.RLock("tpool update")
	defer tp.state.RUnlock("tpool update", counter)

	// Get the block diffs since the previous update.
	var removedBlocks, addedBlocks []consensus.Block
	removedBlocksIDs, addedBlocksIDs, err := tp.state.BlocksSince(tp.recentBlock)
	if err != nil {
		if consensus.DEBUG {
			panic("BlocksSince returned an error?")
		}
		return
	}
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
		// Remove all transactions that have been invalidated by the
		// elimination of this block.
		proofMap, exists := tp.storageProofs[block.ID()]
		if exists {
			for _, ut := range proofMap {
				tp.purgeUnconfirmedTransaction(ut)
			}
		}

		// Add all transactions that got removed to the unconfirmed consensus
		// set.
		for j := len(block.Transactions) - 1; j >= 0; j-- {
			txn := block.Transactions[j]

			// If the transaction contains a storage proof or is non-standard,
			// remove this transaction from the pool. This is done last because
			// we also need to remove any dependents.
			err = tp.IsStandardTransaction(txn)
			if err != nil {
				tp.removeDependentTransactions(txn)
			}

			// set `direction` to false because reversed transactions need to
			// be added to the beginning of the linked list - existing
			// unconfirmed transactions may depend on this rewound transaction.
			direction := false
			tp.addTransactionToPool(txn, direction)
		}
	}

	// Iterate through all of the new transactions and remove them from the
	// transaction pool, also removing any conflicts that have been created.
	for _, block := range addedBlocks {
		for _, txn := range block.Transactions {
			// Determine if this transaction is in the unconfirmed set or not.
			ut, exists := tp.transactions[crypto.HashObject(txn)]
			if exists {
				tp.removeUnconfirmedTransaction(ut)
			} else {
				tp.removeConflictingTransactions(txn)
			}
		}
	}

	// Iterate through all of the unconfirmed file contracts and see if any
	// have been invalidated due to the state height becoming higher than the
	// start height of the contract.
	startingHeight, _ := tp.state.HeightOfBlock(tp.recentBlock)
	for height := startingHeight; height <= tp.state.Height(); height++ {
		invalidContracts, exists := tp.newFileContracts[height]
		if exists {
			// Remove all of the unconfirmed transactions that are now invalid.
			for _, ut := range invalidContracts {
				tp.purgeUnconfirmedTransaction(ut)
			}
		}
	}

	tp.recentBlock = tp.state.CurrentBlock().ID()
}
