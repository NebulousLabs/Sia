package transactionpool

import (
	"github.com/NebulousLabs/Sia/consensus"
)

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

		// Check if this transaction is dependent on another unconfirmed
		// transaction.
		requirement, exists := tp.newSiacoinOutputs[sci.ParentID]
		if exists {
			requirement.dependents[ut] = struct{}{}
		}

		// Add this output to the list of spent outputs.
		tp.usedSiacoinOutputs[sci.ParentID] = ut
	}
}

func (tp *TransactionPool) applySiacoinOutputs(t consensus.Transaction, ut *unconfirmedTransaction) {
	// Add each new siacoin output to the list of siacoinOutputs and newSiacoinOutputs.
	for i, sco := range t.SiacoinOutputs {
		// Sanity check - this output should not already exist in
		// newSiacoinOutputs or siacoinOutputs.
		if consensus.DEBUG {
			_, exists := tp.siacoinOutputs[t.SiacoinOutputID(i)]
			if exists {
				panic("trying to add an output that already exists?")
			}
			_, exists = tp.newSiacoinOutputs[t.SiacoinOutputID(i)]
			if exists {
				panic("trying to add an output that already exists?")
			}
		}

		tp.siacoinOutputs[t.SiacoinOutputID(i)] = sco
		tp.newSiacoinOutputs[t.SiacoinOutputID(i)] = ut
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
		tp.fileContractTerminations[fct.ParentID] = ut
	}
}

func (tp *TransactionPool) applyStorageProofs(t consensus.Transaction, ut *unconfirmedTransaction) {
	for _, sp := range t.StorageProofs {
		// Grab the trigger block.
		fc, _ := tp.state.FileContract(sp.ParentID)
		triggerBlock, exists := tp.state.BlockAtHeight(fc.Start)

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

		// Check if this transaction is dependent on another unconfirmed
		// transaction.
		requirement, exists := tp.newSiafundOutputs[sfi.ParentID]
		if exists {
			requirement.dependents[ut] = struct{}{}
		}

		// Add this output to the list of spent outputs.
		tp.usedSiafundOutputs[sfi.ParentID] = ut
	}
}

func (tp *TransactionPool) applySiafundOutputs(t consensus.Transaction, ut *unconfirmedTransaction) {
	// Add each new siafund output to the list of siafundOutputs and newSiafundOutputs.
	for i, sfo := range t.SiafundOutputs {
		// Sanity check - this output should not already exist in
		// newSiafundOutputs or siafundOutputs.
		if consensus.DEBUG {
			_, exists := tp.siafundOutputs[t.SiafundOutputID(i)]
			if exists {
				panic("trying to add an output that already exists?")
			}
			_, exists = tp.newSiafundOutputs[t.SiafundOutputID(i)]
			if exists {
				panic("trying to add an output that already exists?")
			}
		}

		tp.siafundOutputs[t.SiafundOutputID(i)] = sfo
		tp.newSiafundOutputs[t.SiafundOutputID(i)] = ut
	}
}

func (tp *TransactionPool) addTransactionToPool(t consensus.Transaction) {
	ut := &unconfirmedTransaction{
		transaction: t,
		dependents:  make(map[*unconfirmedTransaction]struct{}),
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

	// Add the transaction.
	tp.addTransactionToPool(t)

	return
}
