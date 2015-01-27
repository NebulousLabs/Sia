package transactionpool

import (
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/hash"
)

var (
	ConflictingTransactionErr = errors.New("conflicting transaction exists within transaction pool")
)

type unconfirmedTransaction struct {
	transaction consensus.Transaction

	newOutputs map[consensus.OutputID]consensus.Output

	requirements []*unconfirmedTransaction
	dependents   []*unconfirmedTransaction
}

type TransactionPool struct {
	state *consensus.State

	usedOutputs map[consensus.OutputID]*unconfirmedTransaction
	newOutputs  map[consensus.OutputID]*unconfirmedTransaction

	storageProofs map[consensus.BlockHeight]map[hash.Hash]consensus.Transaction

	mu sync.RWMutex
}

func (tp *TransactionPool) checkInputs(t consensus.Transaction) (inputSum consensus.Currency, err error) {
	for _, input := range t.Inputs {
		// First check the confirmed output set.
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

			inputSum += output.Value
			continue
		}

		// Second check the unconfirmed output set, and make sure the output is
		// not already in use.
		unconfirmedTxn, existsNew := tp.newOutputs[input.OutputID]
		_, existsUsed := tp.usedOutputs[input.OutputID]
		if existsNew && !existsUsed {
			// Get the output value for the input sum.
			output, exists := unconfirmedTxn.newOutputs[input.OutputID]
			if consensus.DEBUG {
				if !exists {
					panic("inconsistent transaction pool - developer error")
				}
			}

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

			inputSum += output.Value
			continue
		}

		err = errors.New("invalid input in transaction")
		return
	}

	return
}

func (tp *TransactionPool) validTransaction(t consensus.Transaction) (err error) {
	// Get the input sum and verify that all inputs come from a valid source
	// (confirmed or unconfirmed).
	inputSum, err := tp.checkInputs(t)
	if err != nil {
		return
	}

	// Need to get the output sum.
	outputSum := t.OutputSum()
	if inputSum != outputSum {
		err = errors.New("input sum does not equal output sum")
		return
	}

	// Need to do signature validation.
	err = tp.state.ValidSignatures(t)
	if err != nil {
		return
	}

	// Check that all contracts are legal within the existing state.
	for _, contract := range t.FileContracts {
		err = tp.state.ValidContract(contract)
		if err != nil {
			return
		}
	}

	return
}

func (tp *TransactionPool) addTransaction(t consensus.Transaction) {
	ut := &unconfirmedTransaction{
		transaction: t,
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
			unconfirmedTxn.dependents = append(unconfirmedTxn.dependents, ut)
			ut.requirements = append(ut.requirements, unconfirmedTxn)
		}

		tp.usedOutputs[input.OutputID] = ut
	}

	for i, _ := range t.Outputs {
		// Sanity check - this output should not already exist in newOutputs
		if consensus.DEBUG {
			_, exists := tp.newOutputs[t.OutputID(i)]
			if exists {
				panic("trying to add an output that already exists?")
			}
		}

		tp.newOutputs[t.OutputID(i)] = ut
	}
}

func (tp *TransactionPool) AcceptTransaction(t consensus.Transaction) (err error) {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	tp.state.RLock()
	defer tp.state.RUnlock()

	// Check that the transaction follows 'Standard.md' guidelines.
	err = standard(t)
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
