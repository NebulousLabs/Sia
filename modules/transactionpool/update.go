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

// purge removes all transactions from the transaction pool.
func (tp *TransactionPool) purge() {
	// Remove the tail transaction repeatedly until no transactions remain.
	for len(tp.transactionList) != 0 {
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

// ReceiveConsensusSetUpdate gets called to inform the transaction pool of
// changes to the consensus set.
func (tp *TransactionPool) ReceiveConsensusSetUpdate(cc modules.ConsensusChange) {
	lockID := tp.mu.Lock()
	defer tp.mu.Unlock(lockID)

	// Save all of the reverted transactions. Transactions need to appear in
	// 'unconfirmedTxns' in the same order that they would appear in the
	// blockchain. 'revertedBlocks' is backwards (first element has highest
	// height), so each time a new block processed, the transactions need to be
	// prepended to the list of unconfirmed transactions.
	var unconfirmedTxns []types.Transaction
	for _, block := range cc.RevertedBlocks {
		unconfirmedTxns = append(block.Transactions, unconfirmedTxns...)
	}

	// Delete the hashes of each unconfirmed transaction from the 'already
	// seen' list.
	for _, txn := range unconfirmedTxns {
		// Sanity check - transaction should be in the list of already seen
		// transactions.
		if build.DEBUG {
			_, exists := tp.transactions[crypto.HashObject(txn)]
			if !exists {
				panic("transaction should be in the list of already seen transactions")
			}
		}
		delete(tp.transactions, crypto.HashObject(txn))
	}

	// Add all of the current unconfirmed transactions to the unconfirmed
	// transaction list.
	unconfirmedTxns = append(unconfirmedTxns, tp.transactionList...)

	// Purge the pool of unconfirmed transactions so that there is no
	// interference from unconfirmed transactions during the application of
	// potentially conflicting transactions that have been added to the
	// blockchain.
	tp.purge()

	// Apply consensus set diffs and adjust the height.
	tp.applyDiffs(cc.SiacoinOutputDiffs, cc.FileContractDiffs, cc.SiafundOutputDiffs, modules.DiffApply)
	tp.consensusSetHeight -= types.BlockHeight(len(cc.RevertedBlocks))
	tp.consensusSetHeight += types.BlockHeight(len(cc.AppliedBlocks))

	// Mark all of the newly applied transactions as 'already seen'.
	for _, block := range cc.AppliedBlocks {
		for _, txn := range block.Transactions {
			tp.transactions[crypto.HashObject(txn)] = struct{}{}
		}
	}

	// Add all potential unconfirmed transactions back into the pool after
	// checking that they are still valid.
	for _, txn := range unconfirmedTxns {
		// Skip transactions that are now in the consensus set or are otherwise
		// repeats.
		_, exists := tp.transactions[crypto.HashObject(txn)]
		if exists {
			continue
		}

		// Check that the transaction is still valid given the updated
		// consensus set.
		err := tp.validUnconfirmedTransaction(txn)
		if err != nil {
			continue
		}

		// Add the transaction back to the pool.
		tp.addTransactionToPool(txn)
	}

	// Inform subscribers that an update has executed.
	tp.updateSubscribers(cc, tp.transactionList, tp.unconfirmedSiacoinOutputDiffs())
}

// PurgeTransactionPool deletes all transactions from the transaction pool.
// It's a failsafe for when the transaction pool is producing invalid
// transaction sets.
func (tp *TransactionPool) PurgeTransactionPool() {
	lockID := tp.mu.Lock()
	defer tp.mu.Unlock(lockID)
	tp.purge()
}
