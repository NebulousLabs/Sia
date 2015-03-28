package transactionpool

import (
	"fmt"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
)

// removeUnconfirmedTransaction takes an unconfirmed transaction and removes it
// from the transaction pool, but leaves behind all dependencies.
func (tp *TransactionPool) removeUnconfirmedTransaction(ut *unconfirmedTransaction) consensus.Transaction {
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
		fc, exists := tp.referenceFileContracts[fct.ParentID]
		if consensus.DEBUG {
			if !exists {
				panic("cannot locate file contract to delete storage proof transaction")
			}
		}
		delete(tp.fileContractTerminations[fc.Start], fct.ParentID)
		delete(tp.referenceFileContracts, fct.ParentID)
	}
	for _, sp := range t.StorageProofs {
		fc, exists := tp.referenceFileContracts[sp.ParentID]
		if consensus.DEBUG {
			if !exists {
				panic("cannot locate file contract to delete storage proof transaction")
			}
		}
		delete(tp.storageProofsByStart[fc.Start], sp.ParentID)
		delete(tp.storageProofsByExpiration[fc.Expiration], sp.ParentID)
		delete(tp.referenceFileContracts, sp.ParentID)
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
	return t
}

// removeDependentTransactions removes all unconfirmed transactions that are
// dependent on the input transaction.
func (tp *TransactionPool) removeDependentTransactions(t consensus.Transaction) (revertedTxns []consensus.Transaction) {
	for i, _ := range t.SiacoinOutputs {
		dependent, exists := tp.usedSiacoinOutputs[t.SiacoinOutputID(i)]
		if exists {
			revertedTxns = append(revertedTxns, tp.purgeUnconfirmedTransaction(dependent)...)
		}
	}
	for i, fc := range t.FileContracts {
		dependent, exists := tp.fileContractTerminations[fc.Start][t.FileContractID(i)]
		if exists {
			revertedTxns = append(revertedTxns, tp.purgeUnconfirmedTransaction(dependent)...)
		}
	}
	for i, _ := range t.SiafundOutputs {
		dependent, exists := tp.usedSiafundOutputs[t.SiafundOutputID(i)]
		if exists {
			revertedTxns = append(revertedTxns, tp.purgeUnconfirmedTransaction(dependent)...)
		}
	}
	return
}

// purgeUnconfirmedTransaction removes all transactions dependent on the input
// transaction, and then removes the input transaction.
func (tp *TransactionPool) purgeUnconfirmedTransaction(ut *unconfirmedTransaction) (revertedTxns []consensus.Transaction) {
	t := ut.transaction
	revertedTxns = append(revertedTxns, tp.removeDependentTransactions(t)...)
	revertedTxns = append(revertedTxns, tp.removeUnconfirmedTransaction(ut))
	return
}

// removeConflictingTransactions removes all of the transactions that are in
// conflict with the input transaction.
func (tp *TransactionPool) removeConflictingTransactions(t consensus.Transaction) (revertedTxns []consensus.Transaction) {
	for _, sci := range t.SiacoinInputs {
		conflict, exists := tp.usedSiacoinOutputs[sci.ParentID]
		if exists {
			revertedTxns = append(revertedTxns, tp.purgeUnconfirmedTransaction(conflict)...)
		}
	}
	for _, fct := range t.FileContractTerminations {
		// Check for the corresponding file contract.
		fc, exists := tp.referenceFileContracts[fct.ParentID]
		if consensus.DEBUG {
			if !exists {
				panic("could not locate file contract")
			}
		}
		conflict, exists := tp.fileContractTerminations[fc.Start][fct.ParentID]
		if exists {
			revertedTxns = append(revertedTxns, tp.purgeUnconfirmedTransaction(conflict)...)
		}
	}
	for _, sp := range t.StorageProofs {
		fc, exists := tp.referenceFileContracts[sp.ParentID]
		if consensus.DEBUG {
			if !exists {
				panic("could not locate file contract")
			}
		}
		conflict, exists := tp.fileContractTerminations[fc.Start][sp.ParentID]
		if exists {
			revertedTxns = append(revertedTxns, tp.purgeUnconfirmedTransaction(conflict)...)
		}
		conflict, exists = tp.storageProofsByStart[fc.Start][sp.ParentID]
		if exists {
			revertedTxns = append(revertedTxns, tp.purgeUnconfirmedTransaction(conflict)...)
		}
	}
	for _, sfi := range t.SiafundInputs {
		conflict, exists := tp.usedSiafundOutputs[sfi.ParentID]
		if exists {
			revertedTxns = append(revertedTxns, tp.purgeUnconfirmedTransaction(conflict)...)
		}
	}
	return
}

// purge removes all transactions from the transaction pool.
func (tp *TransactionPool) purge() (revertedTxns []consensus.Transaction) {
	for tp.head != nil {
		revertedTxns = append(revertedTxns, tp.purgeUnconfirmedTransaction(tp.head)...)
	}
	return
}

// ReceiveConsensusUpdate gets called any time that consensus changes.
func (tp *TransactionPool) ReceiveConsensusUpdate(revertedBlocks, appliedBlocks []consensus.Block) {
	id := tp.mu.Lock()
	defer tp.mu.Unlock(id)

	// Handle reverted blocks.
	var revertedTxns, appliedTxns []consensus.Transaction
	for _, block := range revertedBlocks {
		// Remove all transactions that have been invalidated by the
		// elimination of this block id - storage proofs are dependent on a
		// specific block id.
		dependentTxns, exists := tp.storageProofsByStart[tp.stateHeight]
		if exists {
			for _, txn := range dependentTxns {
				revertedTxns = append(revertedTxns, tp.purgeUnconfirmedTransaction(txn)...)
			}
		}
		delete(tp.storageProofsByStart, tp.stateHeight)

		// Add all transactions that got removed to the unconfirmed consensus
		// set, add them in reverse order to preserve any dependencies.
		for j := len(block.Transactions) - 1; j >= 0; j-- {
			txn := block.Transactions[j]

			// If the transaction is non-standard, remove its dependencies and
			// don't add it to the pool.
			err := tp.IsStandardTransaction(txn)
			if err != nil {
				revertedTxns = append(revertedTxns, tp.removeDependentTransactions(txn)...)
				continue
			}

			// set `direction` to false because reversed transactions need to
			// be added to the beginning of the linked list - existing
			// unconfirmed transactions may depend on this rewound transaction.
			tp.addTransactionToPool(txn, PriorTransaction)
			appliedTxns = append(appliedTxns, txn)
		}

		tp.stateHeight--
	}

	// Handle applied blocks.
	for _, block := range appliedBlocks {
		tp.stateHeight++

		// Handle any unconfirmed transactions that have been confirmed by this
		// block, and remove any conflicts that have been introduced.
		for _, txn := range block.Transactions {
			ut, exists := tp.transactions[crypto.HashObject(txn)]
			if exists {
				revertedTxns = append(revertedTxns, tp.removeUnconfirmedTransaction(ut))
			} else {
				revertedTxns = append(revertedTxns, tp.removeConflictingTransactions(txn)...)
			}
		}

		// Handle any unconfirmed file contracts that have been invalidated due
		// to the state height increasing.
		expiredTxns, exists := tp.newFileContracts[tp.stateHeight]
		if exists {
			for _, txn := range expiredTxns {
				revertedTxns = append(revertedTxns, tp.purgeUnconfirmedTransaction(txn)...)
			}
		}

		// Handle any storage proofs that have been invalidated because the
		// cooresponding file contract has expired.
		expiredTxns, exists = tp.storageProofsByExpiration[tp.stateHeight]
		if exists {
			for _, txn := range expiredTxns {
				revertedTxns = append(revertedTxns, tp.purgeUnconfirmedTransaction(txn)...)
			}
		}
		delete(tp.storageProofsByExpiration, tp.stateHeight)

		// Handle any terminations that have been invalidated because the
		// corresponding file contract has started.
		expiredTxns, exists = tp.fileContractTerminations[tp.stateHeight]
		if exists {
			for _, txn := range expiredTxns {
				revertedTxns = append(revertedTxns, tp.purgeUnconfirmedTransaction(txn)...)
			}
		}
		delete(tp.fileContractTerminations, tp.stateHeight)
	}

	// Do a purge if the height has decreased after a fork.
	if len(revertedBlocks) > len(appliedBlocks) {
		fmt.Println("Doing a transaction pool purge")
		revertedTxns = append(revertedTxns, tp.purge()...)
	}

	tp.updateSubscribers(revertedBlocks, appliedBlocks, revertedTxns, appliedTxns)
}
