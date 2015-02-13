package transactionpool

import (
	"github.com/NebulousLabs/Sia/consensus"
)

/*
// addTransaction adds a transaction to the transaction pool.
func (tp *TransactionPool) addTransaction(t consensus.Transaction) {
	ut := &unconfirmedTransaction{
		transaction:  t,
		requirements: make(map[*unconfirmedTransaction]struct{}),
		dependents:   make(map[*unconfirmedTransaction]struct{}),
	}

	// Go through the inputs and them to the used outputs list, updating the
	// requirements and dependents as necessary.
	for _, input := range t.SiacoinInputs {
		// Sanity check - this input should not already be in the usedOutputs
		// list.
		if consensus.DEBUG {
			_, exists := tp.usedOutputs[input.ParentID]
			if exists {
				panic("addTransaction called on invalid transaction")
			}
		}

		unconfirmedTxn, exists := tp.newOutputs[input.ParentID]
		if exists {
			unconfirmedTxn.dependents[ut] = struct{}{}
			ut.requirements[unconfirmedTxn] = struct{}{}
		}
		tp.usedOutputs[input.ParentID] = ut
	}

	// Add each new output to the list of outputs and newOutputs.
	for i, output := range t.SiacoinOutputs {
		// Sanity check - this output should not already exist in newOutputs or
		// outputs.
		if consensus.DEBUG {
			_, exists := tp.newOutputs[t.SiacoinOutputID(i)]
			if exists {
				panic("trying to add an output that already exists?")
			}
			_, exists = tp.outputs[t.SiacoinOutputID(i)]
			if exists {
				panic("trying to add an output that already exists?")
			}
		}

		tp.outputs[t.SiacoinOutputID(i)] = output
		tp.newOutputs[t.SiacoinOutputID(i)] = ut
	}

	tp.addTransactionToTail(ut)
}
*/

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
	// tp.addTransactionToPool(t)

	return
}
