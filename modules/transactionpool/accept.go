package transactionpool

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
)

// applySiacoinInputs adds every siacoin input to the transaction pool by
// marking the consumed outputs and pointing to the transaction that consumed
// them.
func (tp *TransactionPool) applySiacoinInputs(t consensus.Transaction, ut *unconfirmedTransaction) {
	// Go through the siacoin inputs and mark them as used.
	for _, sci := range t.SiacoinInputs {
		// Sanity check - this input should not already be in the usedOutputs
		// list.
		if consensus.DEBUG {
			_, exists := tp.usedSiacoinOutputs[sci.ParentID]
			if exists {
				panic("addTransaction called on invalid transaction")
			}
		}

		// Delete the parent output from the output list - this call will work
		// whether the ouput is in the confirmed or unconfirmed set.
		delete(tp.siacoinOutputs, sci.ParentID)

		// Add this output to the list of spent outputs.
		tp.usedSiacoinOutputs[sci.ParentID] = ut
	}
}

// applySiacoinOutputs adds every new siacoin output to the unconfirmed
// consensus set and points to the transaction that created the outputs.
func (tp *TransactionPool) applySiacoinOutputs(t consensus.Transaction, ut *unconfirmedTransaction) {
	// Add each new siacoin output to the list of siacoinOutputs and newSiacoinOutputs.
	for i, sco := range t.SiacoinOutputs {
		// Sanity check - this output should not already exist in
		// newSiacoinOutputs or siacoinOutputs.
		scoid := t.SiacoinOutputID(i)
		if consensus.DEBUG {
			_, exists := tp.siacoinOutputs[scoid]
			if exists {
				panic("trying to add an output that already exists?")
			}
			_, exists = tp.newSiacoinOutputs[scoid]
			if exists {
				panic("trying to add an output that already exists?")
			}
		}

		tp.siacoinOutputs[scoid] = sco
		tp.newSiacoinOutputs[scoid] = ut
	}
}

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
			_, exists = tp.newFileContracts[fc.Start]
			if exists {
				_, exists = tp.newFileContracts[fc.Start][fcid]
				if exists {
					panic("trying to add a file contract that's already recognized")
				}
			}
		}

		// Add the file contract to the unconfirmed set.
		tp.fileContracts[fcid] = fc
		_, exists := tp.newFileContracts[fc.Start]
		if !exists {
			tp.newFileContracts[fc.Start] = make(map[consensus.FileContractID]*unconfirmedTransaction)
		}
		tp.newFileContracts[fc.Start][fcid] = ut
	}
}

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

		// Remove the file contract from the set and add the termination.
		delete(tp.fileContracts, fct.ParentID)

		// Add this termination to the set of terminations.
		tp.fileContractTerminations[fct.ParentID] = ut
	}
}

func (tp *TransactionPool) applyStorageProofs(t consensus.Transaction, ut *unconfirmedTransaction) {
	for _, sp := range t.StorageProofs {
		// Grab the trigger block.
		fc, _ := tp.state.FileContract(sp.ParentID)
		triggerBlock, exists := tp.state.BlockAtHeight(fc.Start - 1)

		// Sanity check - a storage proof for this file contract should not
		// already exist.
		if consensus.DEBUG {
			if !exists {
				panic("invalid storage proof submitted to the transaction pool")
			}

			_, exists = tp.storageProofs[triggerBlock.ID()]
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
		_, exists = tp.storageProofs[triggerBlock.ID()]
		if !exists {
			tp.storageProofs[triggerBlock.ID()] = make(map[consensus.FileContractID]*unconfirmedTransaction)
		}
		tp.storageProofs[triggerBlock.ID()][sp.ParentID] = ut
	}
}

func (tp *TransactionPool) applySiafundInputs(t consensus.Transaction, ut *unconfirmedTransaction) {
	// Go through the siafund inputs and mark them as used.
	for _, sfi := range t.SiafundInputs {
		// Sanity check - this input should not already be in the usedOutputs
		// list.
		if consensus.DEBUG {
			_, exists := tp.usedSiafundOutputs[sfi.ParentID]
			if exists {
				panic("addTransaction called on invalid transaction")
			}
		}

		// Delete the parent output from the output list - this call will work
		// whether the ouput is in the confirmed or unconfirmed set.
		delete(tp.siafundOutputs, sfi.ParentID)

		// Add this output to the list of spent outputs.
		tp.usedSiafundOutputs[sfi.ParentID] = ut
	}
}

func (tp *TransactionPool) applySiafundOutputs(t consensus.Transaction, ut *unconfirmedTransaction) {
	// Add each new siafund output to the list of siafundOutputs and newSiafundOutputs.
	for i, sfo := range t.SiafundOutputs {
		// Sanity check - this output should not already exist in
		// newSiafundOutputs or siafundOutputs.
		sfoid := t.SiafundOutputID(i)
		if consensus.DEBUG {
			_, exists := tp.siafundOutputs[sfoid]
			if exists {
				panic("trying to add an output that already exists?")
			}
			_, exists = tp.newSiafundOutputs[sfoid]
			if exists {
				panic("trying to add an output that already exists?")
			}
		}

		tp.siafundOutputs[sfoid] = sfo
		tp.newSiafundOutputs[sfoid] = ut
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
}

// AcceptTransaction takes a new transaction from the network and puts it in
// the transaction pool after checking it for legality and consistency.
func (tp *TransactionPool) AcceptTransaction(t consensus.Transaction) (err error) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

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

	tp.transactions[crypto.HashObject(t)] = struct{}{}

	return
}
