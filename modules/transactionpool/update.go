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
		// Sanity check - check that the maps are in the expected state.
		if consensus.DEBUG {
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
	for i, _ := range t.SiacoinOutputs {
		scoid := t.SiacoinOutputID(i)
		// Sanity check - output should exist.
		if consensus.DEBUG {
			_, exists := tp.siacoinOutputs[scoid]
			if !exists {
				panic("trying to delete missing siacoin output")
			}
		}
		delete(tp.siacoinOutputs, scoid)
	}
	for i, _ := range t.FileContracts {
		fcid := t.FileContractID(i)
		// Sanity check - file contract should exist.
		if consensus.DEBUG {
			_, exists := tp.fileContracts[fcid]
			if !exists {
				panic("trying to remove missing file contract")
			}
		}
		delete(tp.fileContracts, fcid)
	}
	for _, fct := range t.FileContractTerminations {
		_, exists := tp.referenceFileContracts[fct.ParentID]
		if consensus.DEBUG {
			if !exists {
				panic("cannot locate file contract to delete storage proof transaction")
			}
		}
		tp.fileContracts[fct.ParentID] = tp.referenceFileContracts[fct.ParentID]
		delete(tp.referenceFileContracts, fct.ParentID)
	}
	for _, sp := range t.StorageProofs {
		_, exists := tp.referenceFileContracts[sp.ParentID]
		if consensus.DEBUG {
			if !exists {
				panic("cannot locate file contract to delete storage proof transaction")
			}
		}
		delete(tp.referenceFileContracts, sp.ParentID)
	}
	for _, sfi := range t.SiafundInputs {
		// Sanity check - maps should not reflect reverted siafund input.
		if consensus.DEBUG {
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
	for i, _ := range t.SiafundOutputs {
		sfoid := t.SiafundOutputID(i)
		delete(tp.siafundOutputs, sfoid)
	}
	delete(tp.transactions, crypto.HashObject(t))
	tp.removeUnconfirmedTransactionFromList(ut)
	return
}

// purge removes all transactions from the transaction pool.
func (tp *TransactionPool) purge() {
	for tp.tail != nil {
		tp.removeUnconfirmedTransaction(tp.tail)
	}

	// Sanity check - all reference objects should have been deleted.
	if consensus.DEBUG {
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
	}

	return
}

// applyDiffs takes consensus set diffs and applies them to the unconfirmed
// consensus set.
func (tp *TransactionPool) applyDiffs(scods []consensus.SiacoinOutputDiff, fcds []consensus.FileContractDiff, sfods []consensus.SiafundOutputDiff, dir consensus.DiffDirection) {
	for _, scod := range scods {
		if dir == scod.Direction {
			if consensus.DEBUG {
				_, exists := tp.siacoinOutputs[scod.ID]
				if exists {
					panic("adding an output that already exists")
				}
			}
			tp.siacoinOutputs[scod.ID] = scod.SiacoinOutput
		} else {
			if consensus.DEBUG {
				_, exists := tp.siacoinOutputs[scod.ID]
				if !exists {
					panic("removing an output that doesn't exist")
				}
			}
			delete(tp.siacoinOutputs, scod.ID)
		}
	}
	for _, fcd := range fcds {
		if dir == fcd.Direction {
			if consensus.DEBUG {
				_, exists := tp.fileContracts[fcd.ID]
				if exists {
					panic("adding a contract that already exists")
				}
			}
			tp.fileContracts[fcd.ID] = fcd.FileContract
		} else {
			if consensus.DEBUG {
				_, exists := tp.fileContracts[fcd.ID]
				if !exists {
					panic("removing a contract that doesn't exist")
				}
			}
			delete(tp.fileContracts, fcd.ID)
		}
	}
	for _, sfod := range sfods {
		if dir == sfod.Direction {
			if consensus.DEBUG {
				_, exists := tp.siafundOutputs[sfod.ID]
				if exists {
					panic("adding an output that already exists")
				}
			}
			tp.siafundOutputs[sfod.ID] = sfod.SiafundOutput
		} else {
			if consensus.DEBUG {
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
func (tp *TransactionPool) ReceiveConsensusUpdate(revertedBlocks, appliedBlocks []consensus.Block) {
	id := tp.mu.Lock()
	defer tp.mu.Unlock(id)

	// Grab the set of transactions in the transaction pool, then remove them
	// all. Then apply the consensus updates, then add back whatever
	// transactions are still valid.
	unconfirmedTxnSet := tp.transactionSet()
	tp.purge()

	// Add all transactions that got removed to the transaction pool, so long
	// as they conform to IsStandard rules.
	for i := len(revertedBlocks) - 1; i >= 0; i-- {
		block := revertedBlocks[i]
		for j := len(block.Transactions) - 1; j >= 0; j-- {
			txn := block.Transactions[j]
			err := tp.IsStandardTransaction(txn)
			if err != nil {
				continue
			}
			tp.addTransactionToPool(txn)
		}

		// Add all of the diffs to the unconfirmed set.
		scods, fcds, sfods, _, err := tp.state.BlockDiffs(block.ID())
		if err != nil {
			if consensus.DEBUG {
				panic(err)
			}
		}
		tp.applyDiffs(scods, fcds, sfods, consensus.DiffRevert)

		tp.stateHeight--
	}

	// Handle applied blocks. The state height needs to be incremented at the
	// beginning so that all of the invalidations are looking at the correct
	// height. The diffs need to be applied at the end so that removing
	// unconfirmed transactions don't result in diff conflicts.
	for _, block := range appliedBlocks {
		// Add all of the diffs to the unconfirmed set.
		scods, fcds, sfods, _, err := tp.state.BlockDiffs(block.ID())
		if err != nil {
			if consensus.DEBUG {
				panic(err)
			}
		}
		tp.applyDiffs(scods, fcds, sfods, consensus.DiffApply)

		tp.stateHeight++
	}

	// Add back all of the unconfirmed transactions that are still valid.
	for _, txn := range unconfirmedTxnSet {
		err := tp.validUnconfirmedTransaction(txn)
		if err != nil {
			continue
		}
		tp.addTransactionToPool(txn)
	}

	tp.updateSubscribers(revertedBlocks, appliedBlocks, tp.transactionSet(), tp.unconfirmedSiacoinOutputDiffs())
}
