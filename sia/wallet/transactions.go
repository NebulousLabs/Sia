package wallet

import (
	"errors"
	"strconv"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/signatures"
)

// Reset implements the core.Wallet interface.
func (w *Wallet) Reset() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.spentCounter++
	w.transactions = make(map[string]*openTransaction)

	return nil
}

// RegisterTransaction implements the core.Wallet interface.
func (w *Wallet) RegisterTransaction(t consensus.Transaction) (id string, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	id = strconv.Itoa(w.transactionCounter)
	w.transactionCounter++
	w.transactions[id] = new(openTransaction)
	w.transactions[id].transaction = &t
	return
}

// FundTransaction implements the core.Wallet interface.
func (w *Wallet) FundTransaction(id string, amount consensus.Currency) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Get the transaction.
	ot, exists := w.transactions[id]
	if !exists {
		return errors.New("no transaction of given id found")
	}
	t := ot.transaction

	// Get the set of outputs.
	spendableOutputs, total, err := w.findOutputs(amount)
	if err != nil {
		return err
	}

	// Create and add all of the inputs.
	for _, spendableOutput := range spendableOutputs {
		spendableAddress := w.spendableAddresses[spendableOutput.output.SpendHash]
		newInput := consensus.Input{
			OutputID:        spendableOutput.id,
			SpendConditions: spendableAddress.spendConditions,
		}
		ot.inputs = append(ot.inputs, len(t.Inputs))
		t.Inputs = append(t.Inputs, newInput)
	}

	// Add a refund output if needed.
	if total-amount > 0 {
		coinAddress, _, err := w.coinAddress()

		if err != nil {
			return err
		}
		t.Outputs = append(
			t.Outputs,
			consensus.Output{
				Value:     total - amount,
				SpendHash: coinAddress,
			},
		)
	}
	return nil
}

// AddMinerFee implements the core.Wallet interface.
func (w *Wallet) AddMinerFee(id string, fee consensus.Currency) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	to, exists := w.transactions[id]
	if !exists {
		return errors.New("no transaction found for given id")
	}

	to.transaction.MinerFees = append(to.transaction.MinerFees, fee)
	return nil
}

// AddOutput implements the core.Wallet interface.
func (w *Wallet) AddOutput(id string, o consensus.Output) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	to, exists := w.transactions[id]
	if !exists {
		return errors.New("no transaction found for given id")
	}

	to.transaction.Outputs = append(to.transaction.Outputs, o)
	return nil
}

// AddTimelockedRefund implements the core.Wallet interface.
func (w *Wallet) AddTimelockedRefund(id string, amount consensus.Currency, release consensus.BlockHeight) (spendConditions consensus.SpendConditions, refundIndex uint64, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Get the transaction
	ot, exists := w.transactions[id]
	if !exists {
		err = errors.New("no transaction found for given id")
		return
	}
	t := ot.transaction

	// Get a frozen coin address.
	_, spendConditions, err = w.timelockedCoinAddress(release)
	if err != nil {
		return
	}

	// Add the output to the transaction
	output := consensus.Output{
		Value:     amount,
		SpendHash: spendConditions.CoinAddress(),
	}
	refundIndex = uint64(len(t.Outputs))
	t.Outputs = append(t.Outputs, output)
	return
}

// AddFileContract implements the core.Wallet interface.
func (w *Wallet) AddFileContract(id string, fc consensus.FileContract) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	to, exists := w.transactions[id]
	if !exists {
		return errors.New("no transaction found for given id")
	}

	to.transaction.FileContracts = append(to.transaction.FileContracts, fc)
	return nil
}

// AddStorageProof implements the core.Wallet interface.
func (w *Wallet) AddStorageProof(id string, sp consensus.StorageProof) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	to, exists := w.transactions[id]
	if !exists {
		return errors.New("no transaction found for given id")
	}

	to.transaction.StorageProofs = append(to.transaction.StorageProofs, sp)
	return nil
}

// AddArbitraryData implements the core.Wallet interface.
func (w *Wallet) AddArbitraryData(id string, arb string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	to, exists := w.transactions[id]
	if !exists {
		return errors.New("no transaction found for given id")
	}

	to.transaction.ArbitraryData = append(to.transaction.ArbitraryData, arb)
	return nil
}

// SignTransaction implements the core.Wallet interface.
func (w *Wallet) SignTransaction(id string, wholeTransaction bool) (transaction consensus.Transaction, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Fetch the transaction.
	openTransaction, exists := w.transactions[id]
	if !exists {
		err = errors.New("no transaction found for given id")
		return
	}
	transaction = *openTransaction.transaction

	// Get the coveredfields struct.
	var coveredFields consensus.CoveredFields
	if wholeTransaction {
		coveredFields = consensus.CoveredFields{WholeTransaction: true}
	} else {
		for i := range transaction.MinerFees {
			coveredFields.MinerFees = append(coveredFields.MinerFees, uint64(i))
		}
		for i := range transaction.Inputs {
			coveredFields.Inputs = append(coveredFields.Inputs, uint64(i))
		}
		for i := range transaction.Outputs {
			coveredFields.Outputs = append(coveredFields.Outputs, uint64(i))
		}
		for i := range transaction.FileContracts {
			coveredFields.Contracts = append(coveredFields.Contracts, uint64(i))
		}
		for i := range transaction.StorageProofs {
			coveredFields.StorageProofs = append(coveredFields.StorageProofs, uint64(i))
		}
		for i := range transaction.ArbitraryData {
			coveredFields.ArbitraryData = append(coveredFields.ArbitraryData, uint64(i))
		}

		// TODO: Should we also sign all of the known signatures?
	}

	// For each input in the transaction that we added, provide a signature.
	for _, inputIndex := range openTransaction.inputs {
		input := transaction.Inputs[inputIndex]
		sig := consensus.TransactionSignature{
			InputID:        input.OutputID,
			CoveredFields:  coveredFields,
			PublicKeyIndex: 0,
		}
		transaction.Signatures = append(transaction.Signatures, sig)

		// Hash the transaction according to the covered fields and produce the
		// cryptographic signature.
		secKey := w.spendableAddresses[input.SpendConditions.CoinAddress()].secretKey
		sigHash := transaction.SigHash(len(transaction.Signatures) - 1)
		transaction.Signatures[len(transaction.Signatures)-1].Signature, err = signatures.SignBytes(sigHash[:], secKey)
		if err != nil {
			return
		}

		// Mark the input as spent. Maps :)
		w.spendableAddresses[input.SpendConditions.CoinAddress()].spendableOutputs[input.OutputID].spentCounter = w.spentCounter
	}

	// Delete the open transaction.
	delete(w.transactions, id)

	return
}
