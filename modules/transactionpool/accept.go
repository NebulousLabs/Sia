package transactionpool

import (
	"errors"

	"github.com/NebulousLabs/Sia/consensus"
)

// checkInputs checks that each input spends a valid output that is either in
// the confirmed set of outputs or in the unconfirmed set of outputs.
// checkInputs also returns the sum of all the inputs in the transaction.
func (tp *TransactionPool) checkInputs(t consensus.Transaction) (inputSum consensus.Currency, err error) {
	for _, input := range t.Inputs {
		// Check that this output has not already been spent by an unconfirmed
		// transaction.
		_, exists := tp.outputs[input.OutputID]
		if exists {
			err = errors.New("transaction contains a double-spend")
			return
		}

		// See if the output is in the confirmed set.
		output, exists := tp.state.Output(input.OutputID)
		if exists {
			// Check that the spend conditions of the input match the spend
			// hash of the output, and that the timelock has expired.
			if input.SpendConditions.CoinAddress() != output.SpendHash {
				err = errors.New("invalid input in transaction")
				return
			}
			if input.SpendConditions.TimeLock > tp.state.Height() {
				err = errors.New("invalid input")
				return
			}

			err = inputSum.Add(output.Value)
			if err != nil {
				return
			}
			continue
		}

		// See if the output is in the unconfirmed set.
		output, exists = tp.outputs[input.OutputID]
		if exists {
			// Check that the spend conditions of the input match the spend
			// hash of the output, and that the timelock has expired.
			if input.SpendConditions.CoinAddress() != output.SpendHash {
				err = errors.New("invalid input in transaction")
				return
			}
			if input.SpendConditions.TimeLock > tp.state.Height() {
				err = errors.New("invalid input")
				return
			}

			err = inputSum.Add(output.Value)
			if err != nil {
				return
			}
			continue
		}

		err = errors.New("invalid input in transaction")
		return
	}

	return
}

// validTransaction checks that there are no double spends and that all other
// parts of the transaction are legal according to the Standard.md rules and
// the consensus rules.
func (tp *TransactionPool) validTransaction(t consensus.Transaction) (err error) {
	// Get the input sum and verify that all inputs come from a valid source
	// (confirmed or unconfirmed).
	inputSum, err := tp.checkInputs(t)
	if err != nil {
		return
	}

	// Check that the inputs equal the outputs.
	if inputSum.Cmp(t.OutputSum()) != 0 {
		err = errors.New("input sum does not equal output sum")
		return
	}

	// Check that the signatures are valid.
	err = tp.state.ValidSignatures(t)
	if err != nil {
		return
	}

	// Check that all contracts are legal within the existing state. This means
	// that they have a non-zero payout and that the start date is less than
	// the current height of the blockchain.
	for _, contract := range t.FileContracts {
		err = tp.state.ValidContract(contract)
		if err != nil {
			return
		}
	}

	return
}

// addTransaction adds a transaction to the transaction pool.
func (tp *TransactionPool) addTransaction(t consensus.Transaction) {
	ut := &unconfirmedTransaction{
		transaction:  t,
		requirements: make(map[*unconfirmedTransaction]struct{}),
		dependents:   make(map[*unconfirmedTransaction]struct{}),
	}

	// Go through the inputs and them to the used outputs list, updating the
	// requirements and dependents as necessary.
	for _, input := range t.Inputs {
		// Sanity check - this input should not already be in the usedOutputs
		// list.
		if consensus.DEBUG {
			_, exists := tp.usedOutputs[input.OutputID]
			if exists {
				panic("addTransaction called on invalid transaction")
			}
		}

		unconfirmedTxn, exists := tp.newOutputs[input.OutputID]
		if exists {
			unconfirmedTxn.dependents[ut] = struct{}{}
			ut.requirements[unconfirmedTxn] = struct{}{}
		}
		tp.usedOutputs[input.OutputID] = ut
	}

	// Add each new output to the list of outputs and newOutputs.
	for i, output := range t.Outputs {
		// Sanity check - this output should not already exist in newOutputs or
		// outputs.
		if consensus.DEBUG {
			_, exists := tp.newOutputs[t.OutputID(i)]
			if exists {
				panic("trying to add an output that already exists?")
			}
			_, exists = tp.outputs[t.OutputID(i)]
			if exists {
				panic("trying to add an output that already exists?")
			}
		}

		tp.outputs[t.OutputID(i)] = output
		tp.newOutputs[t.OutputID(i)] = ut
	}

	tp.addTransactionToTail(ut)
}

// AcceptTransaction takes a new transaction from the network and puts it in
// the transaction pool after checking it for legality and consistency.
func (tp *TransactionPool) AcceptTransaction(t consensus.Transaction) (err error) {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	tp.state.RLock()
	defer tp.state.RUnlock()

	// Check that the transaction follows 'Standard.md' guidelines.
	err = tp.IsStandardTransaction(t)
	if err != nil {
		return
	}

	// Handle the transaction differently if it contains a storage proof.
	if len(t.StorageProofs) != 0 {
		err = tp.acceptStorageProofTransaction(t)
		if err != nil {
			return
		}
		return
	}

	// Check that the transaction is legal.
	err = tp.validTransaction(t)
	if err != nil {
		return
	}

	// Add the transaction.
	tp.addTransaction(t)

	return
}
