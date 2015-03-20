package transactionpool

import (
	"errors"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
)

type TransactionDirection bool

const (
	NewTransaction   TransactionDirection = true
	PriorTransaction TransactionDirection = false
)

var (
	ErrDuplicate = errors.New("transaction is a duplicate")
)

// applySiacoinInputs adds every siacoin input to the transaction pool by
// marking the consumed outputs and pointing to the transaction that consumed
// them.
func (tp *TransactionPool) applySiacoinInputs(t consensus.Transaction, ut *unconfirmedTransaction) {
	for _, sci := range t.SiacoinInputs {
		tp.usedSiacoinOutputs[sci.ParentID] = ut
	}
}

// applySiacoinOutputs adds every new siacoin output to the unconfirmed
// consensus set.
func (tp *TransactionPool) applySiacoinOutputs(t consensus.Transaction, ut *unconfirmedTransaction) {
	// Add each new siacoin output to the list of siacoinOutputs and newSiacoinOutputs.
	for i, sco := range t.SiacoinOutputs {
		// Sanity check - output should not exist in the unconfirmed or
		// confirmed set.
		scoid := t.SiacoinOutputID(i)
		if consensus.DEBUG {
			_, exists := tp.siacoinOutputs[scoid]
			if exists {
				panic("trying to add an output that already exists?")
			} else {
				_, exists = tp.state.SiacoinOutput(scoid)
				if exists {
					panic("adding a transaction that's already confirmed")
				}
			}
		}

		tp.siacoinOutputs[scoid] = sco
	}
}

// applyFileContracts adds every file contract in a transaction to the
// unconfirmed set.
func (tp *TransactionPool) applyFileContracts(t consensus.Transaction, ut *unconfirmedTransaction) {
	for i, fc := range t.FileContracts {
		// Sanity check - file contract should not exist in the confirmed or
		// unconfirmed set.
		fcid := t.FileContractID(i)
		if consensus.DEBUG {
			_, exists := tp.fileContracts[fcid]
			if exists {
				panic("trying to add a file contract that's already in the unconfirmed set")
			} else {
				_, exists = tp.state.FileContract(fcid)
				if exists {
					panic("trying to add a file contract that's already in the confirmed set")
				}
			}
		}

		_, exists := tp.newFileContracts[fc.Start]
		if !exists {
			tp.newFileContracts[fc.Start] = make(map[consensus.FileContractID]*unconfirmedTransaction)
		}
		tp.fileContracts[fcid] = fc
		tp.newFileContracts[fc.Start][fcid] = ut
	}
}

// applyFileContractTerminations deletes consumed file contracts from the
// consensus set and points to the transaction that consumed them.
func (tp *TransactionPool) applyFileContractTerminations(t consensus.Transaction, ut *unconfirmedTransaction) {
	for _, fct := range t.FileContractTerminations {
		// Get the file contract to know the starting height.
		fc, exists := tp.fileContracts[fct.ParentID]
		if !exists {
			fc, exists = tp.state.FileContract(fct.ParentID)
			if !exists {
				if consensus.DEBUG {
					panic("misuse of applyFileContractTerminations")
				}
			}
		}

		delete(tp.fileContracts, fct.ParentID)
		tp.fileContractTerminations[fc.Start][fct.ParentID] = ut
		tp.referenceFileContracts[fct.ParentID] = fc
	}
}

// applyStorageProof deletes any file contracts that have been consumed and
// points to the transaction that consumed them. A log is kept of all the
// storage proofs according to their trigger block. The storage proofs are
// removed from the transaction pool if the trigger block changes.
func (tp *TransactionPool) applyStorageProofs(t consensus.Transaction, ut *unconfirmedTransaction) {
	for _, sp := range t.StorageProofs {
		fc, _ := tp.state.FileContract(sp.ParentID)

		// Sanity check - a storage proof for this file contract should not
		// already exist.
		if consensus.DEBUG {
			_, exists := tp.storageProofsByStart[fc.Start]
			if exists {
				_, exists = tp.storageProofsByStart[fc.Start][sp.ParentID]
				if exists {
					panic("storage proof for this contract exists in the by-start map")
				}
			}
			_, exists = tp.storageProofsByExpiration[fc.Expiration]
			if exists {
				_, exists = tp.storageProofsByExpiration[fc.Expiration][sp.ParentID]
				if exists {
					panic("storage proof for this file contract already exists in pool")
				}
			}
		}

		// Add the storage proof to the set of storage proofs.
		_, exists := tp.storageProofsByStart[fc.Start]
		if !exists {
			tp.storageProofsByStart[fc.Start] = make(map[consensus.FileContractID]*unconfirmedTransaction)
		}
		tp.storageProofsByStart[fc.Start][sp.ParentID] = ut

		_, exists = tp.storageProofsByExpiration[fc.Expiration]
		if !exists {
			tp.storageProofsByExpiration[fc.Expiration] = make(map[consensus.FileContractID]*unconfirmedTransaction)
		}
		tp.storageProofsByExpiration[fc.Expiration][sp.ParentID] = ut
		tp.referenceFileContracts[sp.ParentID] = fc
	}
}

// applySiafundInputs marks every siafund output that has been consumed and
// points to the transaction that consumed the output.
func (tp *TransactionPool) applySiafundInputs(t consensus.Transaction, ut *unconfirmedTransaction) {
	for _, sfi := range t.SiafundInputs {
		// Sanity check - this input should not already be in the usedOutputs
		// list.
		if consensus.DEBUG {
			_, exists := tp.usedSiafundOutputs[sfi.ParentID]
			if exists {
				panic("addTransaction called on invalid transaction")
			}
		}

		delete(tp.siafundOutputs, sfi.ParentID)
		tp.usedSiafundOutputs[sfi.ParentID] = ut
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
func (tp *TransactionPool) addTransactionToPool(t consensus.Transaction, source TransactionDirection) {
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
	if source == NewTransaction {
		tp.appendUnconfirmedTransaction(ut)
	} else {
		tp.prependUnconfirmedTransaction(ut)
	}

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
	tp.addTransactionToPool(t, NewTransaction)
	tp.updateSubscribers(nil, nil, nil, []consensus.Transaction{t})

	tp.gateway.RelayTransaction(t) // error is not checked
	return
}
