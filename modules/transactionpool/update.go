package transactionpool

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// update.go listens for changes from the consensus set and integrates them
// into the unconfirmed set. Each time there is a change in the consensus set,
// all transactions are removed from the unconfirmed set, the changes are
// implemented, and then all transactions are verified and then re-added.
// Re-verifying the transactions ensures that no requirements (such as
// expirations and timelocks) are missed, and that no dependencies are missed.
// While computationally expensive, it achieves correctness with less code.

// removeSiacoinInputs removes all of the siacoin inputs of a transaction from
// the unconfirmed consensus set.
func (tp *TransactionPool) removeSiacoinInputs(t types.Transaction) {
	for _, sci := range t.SiacoinInputs {
		// Sanity check - the corresponding output should be in the reference
		// set and absent from the unconfirmed set.
		if build.DEBUG {
			_, exists := tp.referenceSiacoinOutputs[sci.ParentID]
			if !exists {
				panic("unexpected absense of a reference siacoin output")
			}
			_, exists = tp.siacoinOutputs[sci.ParentID]
			if exists {
				panic("unexpected presense of a siacoin output")
			}
		}

		tp.siacoinOutputs[sci.ParentID] = tp.referenceSiacoinOutputs[sci.ParentID]
		delete(tp.referenceSiacoinOutputs, sci.ParentID)
	}
}

// removeSiacoinOutputs removes all of the siacoin outputs of a transaction
// from the unconfirmed consensus set.
func (tp *TransactionPool) removeSiacoinOutputs(t types.Transaction) {
	for i, _ := range t.SiacoinOutputs {
		scoid := t.SiacoinOutputID(i)
		// Sanity check - the output should exist in the unconfirmed set as
		// there should be no transaction dependents who have spent the output.
		if build.DEBUG {
			_, exists := tp.siacoinOutputs[scoid]
			if !exists {
				panic("trying to delete missing siacoin output")
			}
		}

		delete(tp.siacoinOutputs, scoid)
	}
}

// removeFileContracts removes all of the file contracts of a transaction from
// the unconfirmed consensus set.
func (tp *TransactionPool) removeFileContracts(t types.Transaction) {
	for i, _ := range t.FileContracts {
		fcid := t.FileContractID(i)
		// Sanity check - file contract should be in the unconfirmed set as
		// there should be no dependent transactions who have terminated the
		// contract.
		if build.DEBUG {
			_, exists := tp.fileContracts[fcid]
			if !exists {
				panic("trying to remove missing file contract")
			}
		}

		delete(tp.fileContracts, fcid)
	}
}

// removeFileContractTerminations removes all of the file contract terminations
// of a transaction from the unconfirmed consensus set.
func (tp *TransactionPool) removeFileContractTerminations(t types.Transaction) {
	for _, fct := range t.FileContractTerminations {
		// Sanity check - the corresponding file contract should be in the
		// reference set.
		if build.DEBUG {
			_, exists := tp.referenceFileContracts[fct.ParentID]
			if !exists {
				panic("cannot locate file contract to delete storage proof transaction")
			}
		}

		tp.fileContracts[fct.ParentID] = tp.referenceFileContracts[fct.ParentID]
		delete(tp.referenceFileContracts, fct.ParentID)
	}
}

// removeStorageProofs removes all of the storage proofs of a transaction from
// the unconfirmed consensus set.
func (tp *TransactionPool) removeStorageProofs(t types.Transaction) {
	for _, sp := range t.StorageProofs {
		// Sanity check - the corresponding file contract should be in the
		// reference set.
		if build.DEBUG {
			_, exists := tp.referenceFileContracts[sp.ParentID]
			if !exists {
				panic("cannot locate file contract to delete storage proof transaction")
			}
		}

		tp.fileContracts[sp.ParentID] = tp.referenceFileContracts[sp.ParentID]
		delete(tp.referenceFileContracts, sp.ParentID)
	}
}

// removeSiafundInputs removes all of the siafund inputs of a transaction from
// the unconfirmed consensus set.
func (tp *TransactionPool) removeSiafundInputs(t types.Transaction) {
	for _, sfi := range t.SiafundInputs {
		// Sanity check - the corresponding siafund output should be in the
		// reference set and absent from the unconfirmed set.
		if build.DEBUG {
			_, exists := tp.siafundOutputs[sfi.ParentID]
			if exists {
				panic("trying to add back existing siafund output")
			}
			_, exists = tp.referenceSiafundOutputs[sfi.ParentID]
			if !exists {
				panic("trying to remove missing reference siafund output")
			}
		}

		tp.siafundOutputs[sfi.ParentID] = tp.referenceSiafundOutputs[sfi.ParentID]
		delete(tp.referenceSiafundOutputs, sfi.ParentID)
	}
}

// removeSiafundOutputs removes all of the siafund outputs of a transaction
// from the unconfirmed consensus set.
func (tp *TransactionPool) removeSiafundOutputs(t types.Transaction) {
	for i, _ := range t.SiafundOutputs {
		// Sanity check - the output should exist in the unconfirmed set as
		// there is no dependent transaction which could have spent the output.
		sfoid := t.SiafundOutputID(i)
		if build.DEBUG {
			_, exists := tp.siafundOutputs[sfoid]
			if !exists {
				panic("trying to remove nonexisting siafund output from unconfirmed set")
			}
		}

		delete(tp.siafundOutputs, sfoid)
	}
}

// removeTailTransaction removes the most recent transaction from the pool. The
// most recent transaction is guaranteed not to have any dependents or
// children.
func (tp *TransactionPool) removeTailTransaction() {
	// Sanity check - the transaction list should not be empty if
	// removeTailTransaction has been called.
	if len(tp.transactionList) == 0 {
		if build.DEBUG {
			panic("calling removeTailTransaction when transaction list is empty")
		}
		return
	}

	// Grab the most recent transaction and remove it from the unconfirmed
	// consensus set piecemeal.
	t := tp.transactionList[len(tp.transactionList)-1]
	tp.removeSiacoinInputs(t)
	tp.removeSiacoinOutputs(t)
	tp.removeFileContracts(t)
	tp.removeFileContractTerminations(t)
	tp.removeStorageProofs(t)
	tp.removeSiafundInputs(t)
	tp.removeSiafundOutputs(t)

	// Remove the transaction from the transaction lists.
	delete(tp.transactions, crypto.HashObject(t))
	tp.transactionList = tp.transactionList[:len(tp.transactionList)-1]

	// Sanity check - the lengths of the transactions by hash vs. the ordered
	// set of transactions should always be the same.
	if len(tp.transactions) != len(tp.transactionList) {
		panic("length mismatch for transactions and transactionList")
	}

	return
}

// purge removes all transactions from the transaction pool.
func (tp *TransactionPool) purge() {
	// Remove the tail transaction repeatedly until no transactions remain.
	for len(tp.transactions) != 0 {
		tp.removeTailTransaction()
	}

	// Sanity check - all reference objects should have been deleted, and the
	// list of unconfirmed transactions should be empty.
	if build.DEBUG {
		if len(tp.referenceSiacoinOutputs) != 0 {
			panic("referenceSiacoinOutputs is not empty")
		}
		if len(tp.referenceFileContracts) != 0 {
			panic("referenceFileContracts is not empty")
		}
		if len(tp.referenceSiafundOutputs) != 0 {
			panic("referenceSiafundOuptuts is not empty")
		}
		if len(tp.transactions) != 0 {
			panic("transactions is not empty")
		}
		if len(tp.transactionList) != 0 {
			panic("transactionList is not empty")
		}
	}

	return
}

// applyDiffs takes a set of diffs from a block and applies them to the
// unconfirmed consensus set.
func (tp *TransactionPool) applyDiffs(scods []modules.SiacoinOutputDiff, fcds []modules.FileContractDiff, sfods []modules.SiafundOutputDiff, dir modules.DiffDirection) {
	// If the block is being reverted, the diffs need to be reverted in the
	// reverse order that they were applied.
	if dir == modules.DiffRevert {
		var tmpScods []modules.SiacoinOutputDiff
		for i := len(scods) - 1; i >= 0; i-- {
			tmpScods = append(tmpScods, scods[i])
		}
		scods = tmpScods

		var tmpFcds []modules.FileContractDiff
		for i := len(fcds) - 1; i >= 0; i-- {
			tmpFcds = append(tmpFcds, fcds[i])
		}
		fcds = tmpFcds

		var tmpSfods []modules.SiafundOutputDiff
		for i := len(sfods) - 1; i >= 0; i-- {
			tmpSfods = append(tmpSfods, sfods[i])
		}
		sfods = tmpSfods
	}

	// Apply all of the siacoin output changes.
	for _, scod := range scods {
		if dir == scod.Direction {
			if build.DEBUG {
				_, exists := tp.siacoinOutputs[scod.ID]
				if exists {
					panic("adding an output that already exists")
				}
			}
			tp.siacoinOutputs[scod.ID] = scod.SiacoinOutput
		} else {
			if build.DEBUG {
				_, exists := tp.siacoinOutputs[scod.ID]
				if !exists {
					panic("removing an output that doesn't exist")
				}
			}
			delete(tp.siacoinOutputs, scod.ID)
		}
	}

	// Apply all of the file contract changes.
	for _, fcd := range fcds {
		if dir == fcd.Direction {
			if build.DEBUG {
				_, exists := tp.fileContracts[fcd.ID]
				if exists {
					panic("adding a contract that already exists")
				}
			}
			tp.fileContracts[fcd.ID] = fcd.FileContract
		} else {
			if build.DEBUG {
				_, exists := tp.fileContracts[fcd.ID]
				if !exists {
					panic("removing a contract that doesn't exist")
				}
			}
			delete(tp.fileContracts, fcd.ID)
		}
	}

	// Apply all of the siafund output changes.
	for _, sfod := range sfods {
		if dir == sfod.Direction {
			if build.DEBUG {
				_, exists := tp.siafundOutputs[sfod.ID]
				if exists {
					panic("adding an output that already exists")
				}
			}
			tp.siafundOutputs[sfod.ID] = sfod.SiafundOutput
		} else {
			if build.DEBUG {
				_, exists := tp.siafundOutputs[sfod.ID]
				if !exists {
					panic("removing an output that doesn't exist")
				}
			}
			delete(tp.siafundOutputs, sfod.ID)
		}
	}
}

// ReceiveConsensusUpdate gets called to inform the transaction pool of changes
// to the consensus set.
func (tp *TransactionPool) ReceiveConsensusUpdate(revertedBlocks, appliedBlocks []types.Block) {
	id := tp.mu.Lock()
	defer tp.mu.Unlock(id)

	// Save all of the reverted transactions. When the existing unconfirmed
	// transactions are added back to the pool, these previously confirmed
	// transactions will also be added to the pool. Because the transactions
	// get added in order, they need to be saved in order. After grabbing the
	// set of reverted transactions, also grab the set of unconfirmed
	// transactions.
	var unconfirmedTxns []types.Transaction
	for _, block := range revertedBlocks {
		unconfirmedTxns = append(unconfirmedTxns, block.Transactions...)
	}
	unconfirmedTxns = append(unconfirmedTxns, tp.transactionList...)

	// Purge the pool of unconfirmed transactions so that there is no
	// interference from unconfirmed transactions during the application of
	// potentially conflicting transactions that have been added to the
	// blockchain.
	tp.purge()

	// Apply all of the reverted diffs to the unconfirmed set. The diffs need
	// to be applied in the inverse order of how they were applied.
	for i := len(revertedBlocks) - 1; i >= 0; i-- {
		block := revertedBlocks[i]
		scods, fcds, sfods, _, err := tp.consensusSet.BlockDiffs(block.ID())
		if err != nil {
			if build.DEBUG {
				panic(err)
			}
		}
		tp.applyDiffs(scods, fcds, sfods, modules.DiffRevert)

		tp.consensusSetHeight--
	}

	// Handle applied blocks. The consensus set height needs to be incremented
	// at the beginning so that all of the invalidations are looking at the
	// correct height. The diffs need to be applied at the end so that removing
	// unconfirmed transactions don't result in diff conflicts.
	for _, block := range appliedBlocks {
		// Add all of the diffs to the unconfirmed set.
		scods, fcds, sfods, _, err := tp.consensusSet.BlockDiffs(block.ID())
		if err != nil {
			if build.DEBUG {
				panic(err)
			}
		}
		tp.applyDiffs(scods, fcds, sfods, modules.DiffApply)

		tp.consensusSetHeight++
	}

	// Add all potential unconfirmed transactions back into the pool after
	// checking that they are still valid.
	for _, txn := range unconfirmedTxns {
		err := tp.validUnconfirmedTransaction(txn)
		if err != nil {
			continue
		}
		tp.addTransactionToPool(txn)
	}

	// Inform the subscribers that an update has executed.
	tp.updateSubscribers(revertedBlocks, appliedBlocks, tp.transactionList, tp.unconfirmedSiacoinOutputDiffs())
}
