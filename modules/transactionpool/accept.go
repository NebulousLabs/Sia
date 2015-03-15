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
// them.
func (tp *TransactionPool) applySiacoinInputs(t consensus.Transaction, ut *unconfirmedTransaction) {
	for _, sci := range t.SiacoinInputs {
		// Sanity check - this input should not already be in the usedOutputs
		// list.
		if consensus.DEBUG {
			_, exists := tp.usedSiacoinOutputs[sci.ParentID]
			if exists {
				panic("addTransaction called on invalid transaction")
			}
		}

		// Add this output to the list of spent outputs.
		tp.usedSiacoinOutputs[sci.ParentID] = ut
	}
}

// applySiacoinOutputs adds every new siacoin output to the unconfirmed
// consensus set.
func (tp *TransactionPool) applySiacoinOutputs(t consensus.Transaction, ut *unconfirmedTransaction) {
	// Add each new siacoin output to the list of siacoinOutputs and newSiacoinOutputs.
	for i, sco := range t.SiacoinOutputs {
		// Sanity check - this output should not already exist in
		// siacoinOutputs.
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

// applyFileContracts scans a transaction for outputs and adds every new file
// contract and adds the contracts to the unconfirmed consensus set. A log is
// kept of the file contracts according to the start height. If the blockchain
// reaches that height, the transaction is removed from the pool because it
// will no longer be valid.
func (tp *TransactionPool) applyFileContracts(t consensus.Transaction, ut *unconfirmedTransaction) {
	for i, fc := range t.FileContracts {
		// Sanity check - this file contract should not already be in the list
		// of unconfirmed file contracts.
		fcid := t.FileContractID(i)
		if consensus.DEBUG {
			_, exists := tp.fileContracts[fcid]
			if exists {
				panic("trying to add a file contract that's already in the unconfirmed set")
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
// consensus set and pints to the transaction that consumed them.
func (tp *TransactionPool) applyFileContractTerminations(t consensus.Transaction, ut *unconfirmedTransaction) {
	for _, fct := range t.FileContractTerminations {
		// Sanity check - this termination should not already be in the list of
		// contract terminations.
		if consensus.DEBUG {
			_, exists := tp.fileContractTerminations[fct.ParentID]
			if exists {
				panic("trying to terminate a file contract that has already been terminated")
			}
		}

		delete(tp.fileContracts, fct.ParentID)
		tp.fileContractTerminations[fct.ParentID] = ut
	}
}

// applyStorageProof deletes any file contracts that have been consumed and
// points to the transaction that consumed them. A log is kept of all the
// storage proofs according to their trigger block. The storage proofs are
// removed from the transaction pool if the trigger block changes.
func (tp *TransactionPool) applyStorageProofs(t consensus.Transaction, ut *unconfirmedTransaction) {
	for _, sp := range t.StorageProofs {
		// Grab the trigger block.
		fc, _ := tp.state.FileContract(sp.ParentID)
		triggerBlock, _ := tp.state.BlockAtHeight(fc.Start - 1)

		// Sanity check - a storage proof for this file contract should not
		// already exist.
		if consensus.DEBUG {
			_, exists := tp.storageProofs[triggerBlock.ID()]
			if exists {
				_, exists = tp.storageProofs[triggerBlock.ID()][sp.ParentID]
				if exists {
					panic("storage proof for this file contract already exists in pool")
				}
			}
		}

		// Remove the file contract from the set and add the termination.
		delete(tp.fileContracts, sp.ParentID)

		// Add the storage proof to the set of storage proofs.
		_, exists := tp.storageProofs[triggerBlock.ID()]
		if !exists {
			tp.storageProofs[triggerBlock.ID()] = make(map[consensus.FileContractID]*unconfirmedTransaction)
		}
		tp.storageProofs[triggerBlock.ID()][sp.ParentID] = ut
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
func (tp *TransactionPool) addTransactionToPool(t consensus.Transaction, direction bool) {
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
	if direction {
		tp.appendUnconfirmedTransaction(ut)
	} else {
		tp.prependUnconfirmedTransaction(ut)
	}

	tp.transactions[crypto.HashObject(t)] = ut
}

// AcceptTransaction takes a new transaction from the network and puts it in
// the transaction pool after checking it for legality and consistency.
func (tp *TransactionPool) AcceptTransaction(t consensus.Transaction) (err error) {
	// Check that the transaction has not been seen before.
	id := tp.mu.Lock()
	txnHash := crypto.HashObject(t)
	_, exists := tp.seenTransactions[txnHash]
	if exists {
		tp.mu.Unlock(id)
		return ErrDuplicate
	}
	if len(tp.seenTransactions) > 1200 {
		tp.seenTransactions = make(map[crypto.Hash]struct{})
	}
	tp.seenTransactions[txnHash] = struct{}{}

	// Check that the transaction is legal given the consensus set of the state
	// and the unconfirmed set of the transaction pool.
	err = tp.validUnconfirmedTransaction(t)
	if err != nil {
		return
	}

	// direction is set to true because a new transaction has been added and it
	// may depend on existing unconfirmed transactions.
	direction := true
	tp.addTransactionToPool(t, direction)
	tp.updateSubscribers(nil, nil, nil, []consensus.Transaction{t})
	tp.mu.Unlock(id)

	tp.gateway.RelayTransaction(t) // error is not checked
	return
}
