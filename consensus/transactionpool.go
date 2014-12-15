package consensus

// TransactionPoolDump() returns the list of transactions that are valid but
// haven't yet appeared in a block. It performs a safety/sanity check to verify
// that no bad transactions have snuck in.
func (s *State) TransactionPoolDump() (transactions []Transaction) {
	for _, transaction := range s.transactionList {
		// Sanity check: make sure each transaction being dumped is valid.
		err := s.ValidTransaction(*transaction)
		if err != nil {
			panic(err)
		}

		transactions = append(transactions, *transaction)
	}

	return
}

// State.addTransactionToPool() adds a transaction to the transaction pool and
// transaction list. A panic will trigger if there is a conflicting transaction
// in the pool.
func (s *State) addTransactionToPool(t *Transaction) {
	// Safety check - there must be no conflict with any inputs that exists in
	// the transaciton list.
	_, exists := s.transactionList[t.Inputs[0].OutputID]
	if exists {
		panic("tring to add an in-conflict transaction to the transaction list")
	}

	// Add each input to the transaction pool.
	for _, input := range t.Inputs {
		// Safety check - there must be no conflict with any inputs that exists
		// in the transaciton pool.
		_, exists := s.transactionPoolOutputs[input.OutputID]
		if exists {
			panic("trying to add an in-conflict transaction to the transaction pool.")
		}

		s.transactionPoolOutputs[input.OutputID] = t
	}

	// Add each proof to the transaction pool.
	for _, proof := range t.StorageProofs {
		// Safety check - must be no existing conflicts.
		_, exists := s.transactionPoolProofs[proof.ContractID]
		if exists {
			panic("trying to add an in-conflict storage proof.")
		}

		s.transactionPoolProofs[proof.ContractID] = t
	}

	// Add the first input to the transaction list.
	s.transactionList[t.Inputs[0].OutputID] = t
}

// Removes a particular transaction from the transaction pool. The transaction
// must already be in the pool or a panic will trigger.
func (s *State) removeTransactionFromPool(t *Transaction) {
	// Safety check - the transaction must already exist within the transaction
	// list.
	_, exists := s.transactionList[t.Inputs[0].OutputID]
	if !exists {
		panic("trying to delete a transaction from transaction list that already does not exists.")
	}

	// Remove each input from the transaction pool.
	for _, input := range t.Inputs {
		// Safety check - the input must already exist.
		_, exists := s.transactionPoolOutputs[input.OutputID]
		if !exists {
			panic("trying to delete a transaction input from the transaction pool that already does not exist.")
		}

		delete(s.transactionPoolOutputs, input.OutputID)
	}

	// Remove each storage proof from the transaction pool.
	for _, proof := range t.StorageProofs {
		// Safety check - the proof must already exist.
		_, exists := s.transactionPoolProofs[proof.ContractID]
		if !exists {
			panic("trying to delete a transaction proof from the pool that already does not exist.")
		}

		delete(s.transactionPoolProofs, proof.ContractID)
	}

	// Remove the transaction from the transaction list.
	delete(s.transactionList, t.Inputs[0].OutputID)
}

// removeTransactionConflictsFromPool removes all transactions from the
// transaction pool that are in conflict with 't', called when 't' is in a
// block.
func (s *State) removeTransactionConflictsFromPool(t *Transaction) {
	// For each input, see if there's a conflicting transaction and if there
	// is, remove the conflicting transaction.
	for _, input := range t.Inputs {
		conflict, exists := s.transactionPoolOutputs[input.OutputID]
		if exists {
			s.removeTransactionFromPool(conflict)
		}
	}

	// For each storage proof, see if there's a conflict and remove the
	// conflicting transaction if there is.
	for _, proof := range t.StorageProofs {
		conflict, exists := s.transactionPoolProofs[proof.ContractID]
		if exists {
			s.removeTransactionFromPool(conflict)
		}
	}
}

// cleanTransactionPool removes transactions from the pool that are no longer
// valid. This can happen if a proof of storage window index changes before the
// proof makes it into a block. Can also happen during reorgs.
func (s *State) cleanTransactionPool() {
	var badTransactions []*Transaction
	for _, transaction := range s.transactionList {
		err := s.ValidTransaction(*transaction)
		if err != nil {
			badTransactions = append(badTransactions, transaction)
		}
	}
	for _, transaction := range badTransactions {
		s.removeTransactionFromPool(transaction)
	}

	// Once you've switched to ValidFloatingTransaction(), need to do a
	// cascading removal of all transactions dependant on any that got removed.
}

// transactionPoolConflict compares a transaction to the transaction pool and
// returns true if there is already a transaction in the transaction pool that
// is in conflict with the current transaction.
func (s *State) transactionPoolConflict(t *Transaction) (conflict bool) {
	// Check for input conflicts.
	for _, input := range t.Inputs {
		_, exists := s.transactionPoolOutputs[input.OutputID]
		if exists {
			conflict = true
		}
	}

	// Check for storage proof conflicts, there can only storage proof for each
	// contract + window index.
	for _, proof := range t.StorageProofs {
		_, exists := s.transactionPoolProofs[proof.ContractID]
		if exists {
			conflict = true
		}
	}

	return
}
