package transactionpool

import (
	"errors"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
)

var (
	ErrDuplicate = errors.New("transaction is a duplicate")
)

// applySiacoinInputs adds every siacoin input to the transaction pool by
// marking the consumed outputs and pointing to the transaction that consumed
// them
func (tp *TransactionPool) applySiacoinInputs(t consensus.Transaction, ut *unconfirmedTransaction) {
	// For each siacoin input, remove the output from the unconfirmed set and
	// add the output to the reference set.
	for _, sci := range t.SiacoinInputs {
		// Sanity check - the maps should not yet be aware that this output has
		// been spent.
		if consensus.DEBUG {
			_, exists := tp.referenceSiacoinOutputs[sci.ParentID]
			if exists {
				panic("applying a siacoin output that's already in the reference set")
			}
			_, exists = tp.siacoinOutputs[sci.ParentID]
			if !exists {
				panic("applying a siacoin input that spends an unrecognized siacoin output")
			}
		}

		tp.referenceSiacoinOutputs[sci.ParentID] = tp.siacoinOutputs[sci.ParentID]
		delete(tp.siacoinOutputs, sci.ParentID)
	}
}

// applySiacoinOutputs adds every new siacoin output to the unconfirmed
// consensus set.
func (tp *TransactionPool) applySiacoinOutputs(t consensus.Transaction, ut *unconfirmedTransaction) {
	// Add each new siacoin output to the list of siacoinOutputs and newSiacoinOutputs.
	for i, sco := range t.SiacoinOutputs {
		// Sanity check - output should not exist in the unconfirmed set.
		scoid := t.SiacoinOutputID(i)
		if consensus.DEBUG {
			_, exists := tp.siacoinOutputs[scoid]
			if exists {
				panic("trying to add an output that already exists?")
			}
		}

		tp.siacoinOutputs[scoid] = sco
	}
}

// applyFileContracts adds every file contract in a transaction to the
// unconfirmed set.
func (tp *TransactionPool) applyFileContracts(t consensus.Transaction, ut *unconfirmedTransaction) {
	for i, fc := range t.FileContracts {
		// Sanity check - file contract should be in the unconfirmed set.
		fcid := t.FileContractID(i)
		if consensus.DEBUG {
			_, exists := tp.fileContracts[fcid]
			if exists {
				panic("trying to add a file contract that's already in the unconfirmed set")
			}
		}

		tp.fileContracts[fcid] = fc
	}
}

// applyFileContractTerminations deletes consumed file contracts from the
// consensus set and points to the transaction that consumed them.
func (tp *TransactionPool) applyFileContractTerminations(t consensus.Transaction, ut *unconfirmedTransaction) {
	for _, fct := range t.FileContractTerminations {
		// Sanity check - maps should not already represent the changes.
		fc, exists := tp.fileContracts[fct.ParentID]
		if consensus.DEBUG {
			if !exists {
				panic("could not find file contract")
			}
			_, exists = tp.referenceFileContracts[fct.ParentID]
			if exists {
				panic("reference contract already exists")
			}
		}

		delete(tp.fileContracts, fct.ParentID)
		tp.referenceFileContracts[fct.ParentID] = fc
	}
}

// applyStorageProof deletes any file contracts that have been consumed and
// points to the transaction that consumed them. A log is kept of all the
// storage proofs according to their trigger block. The storage proofs are
// removed from the transaction pool if the trigger block changes.
func (tp *TransactionPool) applyStorageProofs(t consensus.Transaction, ut *unconfirmedTransaction) {
	for _, sp := range t.StorageProofs {
		// Sanity check - maps should not yet represent changes.
		fc, exists := tp.fileContracts[sp.ParentID]
		if consensus.DEBUG {
			if !exists {
				panic("could not find file contract in unconfirmed set")
			}
			_, exists = tp.referenceFileContracts[sp.ParentID]
			if exists {
				panic("file contract is in both unconfirmed set and reference set")
			}
		}

		// Add the storage proof to the set of storage proofs.
		tp.referenceFileContracts[sp.ParentID] = fc
		delete(tp.fileContracts, sp.ParentID)
	}
}

// applySiafundInputs marks every siafund output that has been consumed and
// points to the transaction that consumed the output.
func (tp *TransactionPool) applySiafundInputs(t consensus.Transaction, ut *unconfirmedTransaction) {
	for _, sfi := range t.SiafundInputs {
		// Sanity check - maps should not yet reflect changes.
		if consensus.DEBUG {
			_, exists := tp.referenceSiafundOutputs[sfi.ParentID]
			if exists {
				panic("applying a siafund output that's already in the reference set")
			}
			_, exists = tp.siafundOutputs[sfi.ParentID]
			if !exists {
				panic("applying a siafund input that spends an unrecognized siafund output")
			}
		}

		tp.referenceSiafundOutputs[sfi.ParentID] = tp.siafundOutputs[sfi.ParentID]
		delete(tp.siafundOutputs, sfi.ParentID)
	}
}

// applySiafundOutputs adds all of the siafund outputs to the unconfirmed
// consensus set.
func (tp *TransactionPool) applySiafundOutputs(t consensus.Transaction, ut *unconfirmedTransaction) {
	// Add each new siafund output to the list of siafundOutputs and newSiafundOutputs.
	for i, sfo := range t.SiafundOutputs {
		// Sanity check - this output should not already exist in the
		// unconfirmed consensus set.
		sfoid := t.SiafundOutputID(i)
		if consensus.DEBUG {
			_, exists := tp.siafundOutputs[sfoid]
			if exists {
				panic("trying to add an output that already exists?")
			}
		}

		tp.siafundOutputs[sfoid] = sfo
	}
}

// addTransactionToPool takes a transaction and creates an
// unconfirmedTransaction object for the transaction, updating the pool to
// indicate the resources that have been created and consumed. Then the
// unconfirmedTransaction is appended or prepended to the linked list of
// transactions depending on the value of `direction`, false means prepend,
// true means append.
func (tp *TransactionPool) addTransactionToPool(t consensus.Transaction) {
	ut := &unconfirmedTransaction{
		transaction: t,
	}

	// Apply each individual part of the transaction to the transaction pool.
	tp.applySiacoinInputs(t, ut)
	tp.applySiacoinOutputs(t, ut)
	tp.applyFileContracts(t, ut)
	tp.applyFileContractTerminations(t, ut)
	tp.applyStorageProofs(t, ut)
	tp.applySiafundInputs(t, ut)
	tp.applySiafundOutputs(t, ut)

	// Add the unconfirmed transaction to the end of the linked list of
	// transactions.
	tp.appendUnconfirmedTransaction(ut)
	tp.transactions[crypto.HashObject(t)] = ut
}

// AcceptTransaction takes a new transaction from the network and puts it in
// the transaction pool after checking it for legality and consistency.
func (tp *TransactionPool) AcceptTransaction(t consensus.Transaction) (err error) {
	id := tp.mu.Lock()
	defer tp.mu.Unlock(id)

	// Check that the transaction has not been seen before.
	txnHash := crypto.HashObject(t)
	_, exists := tp.transactions[txnHash]
	if exists {
		return ErrDuplicate
	}

	// Check that the transaction is legal given the consensus set of the state
	// and the unconfirmed set of the transaction pool.
	err = tp.validUnconfirmedTransaction(t)
	if err != nil {
		return
	}

	// direction is set to true because a new transaction has been added and it
	// may depend on existing unconfirmed transactions.
	tp.addTransactionToPool(t)
	tp.updateSubscribers(nil, nil, tp.transactionSet(), tp.unconfirmedSiacoinOutputDiffs())

	tp.gateway.RelayTransaction(t) // error is not checked
	return
}
