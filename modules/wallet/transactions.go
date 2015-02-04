package wallet

import (
	"errors"
	"strconv"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
)

// openTransaction is a type that the wallet uses to track a transaction as it
// adds inputs and other features. `inputs` is a list of inputs (their indicies
// in the transaction) that the wallet has added personally, so that the inputs
// can be signed when SignTransaction() is called.
type openTransaction struct {
	transaction *consensus.Transaction
	inputs      []int
}

// RegisterTransaction starts with a transaction as input and adds that
// transaction to the list of open transactions, returning an id. That id can
// then be used to modify and sign the transaction. An empty transaction is
// legal input.
func (w *Wallet) RegisterTransaction(t consensus.Transaction) (id string, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	id = strconv.Itoa(w.transactionCounter)
	w.transactionCounter++
	w.transactions[id] = new(openTransaction)
	w.transactions[id].transaction = &t
	return
}

// FundTransaction adds enough inputs to equal `amount` of input to the
// transaction, and also adds any refunds necessary to get the balance correct.
func (w *Wallet) FundTransaction(id string, amount consensus.Currency) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Get the transaction.
	openTxn, exists := w.transactions[id]
	if !exists {
		return errors.New("no transaction of given id found")
	}
	txn := openTxn.transaction

	// Get the set of outputs to use as inputs.
	knownOutputs, total, err := w.findOutputs(amount)
	if err != nil {
		return err
	}

	// Create and add all of the inputs.
	for _, knownOutput := range knownOutputs {
		key := w.keys[knownOutput.output.SpendHash]
		newInput := consensus.Input{
			OutputID:        knownOutput.id,
			SpendConditions: key.spendConditions,
		}
		openTxn.inputs = append(openTxn.inputs, len(txn.Inputs))
		txn.Inputs = append(txn.Inputs, newInput)

		// Set the age of the knownOutput to prevent accidental double spends.
		knownOutput.age = w.age
	}

	// Add a refund output if needed.
	refund := total
	err = refund.Sub(amount)
	if err != nil && !refund.IsZero() {
		coinAddress, _, err := w.coinAddress()
		if err != nil {
			return err
		}

		txn.Outputs = append(
			txn.Outputs,
			consensus.Output{
				Value:     refund,
				SpendHash: coinAddress,
			},
		)
	}
	return nil
}

// AddMinerFee will add a miner fee to the transaction, but will not add any
// inputs.
func (w *Wallet) AddMinerFee(id string, fee consensus.Currency) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	openTxn, exists := w.transactions[id]
	if !exists {
		return errors.New("no transaction found for given id")
	}

	openTxn.transaction.MinerFees = append(openTxn.transaction.MinerFees, fee)
	return nil
}

// AddOutput adds an output to the transaction, but will not add any inputs.
// It returns the index of the output in the transaction.
func (w *Wallet) AddOutput(id string, output consensus.Output) (index uint64, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	openTxn, exists := w.transactions[id]
	if !exists {
		err = errors.New("no transaction found for given id")
		return
	}

	openTxn.transaction.Outputs = append(openTxn.transaction.Outputs, output)
	index = uint64(len(openTxn.transaction.Outputs) - 1)
	return
}

// AddFileContract implements the core.Wallet interface.
func (w *Wallet) AddFileContract(id string, fc consensus.FileContract) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	openTxn, exists := w.transactions[id]
	if !exists {
		return errors.New("no transaction found for given id")
	}

	openTxn.transaction.FileContracts = append(openTxn.transaction.FileContracts, fc)
	return nil
}

// AddStorageProof implements the core.Wallet interface.
func (w *Wallet) AddStorageProof(id string, sp consensus.StorageProof) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	openTxn, exists := w.transactions[id]
	if !exists {
		return errors.New("no transaction found for given id")
	}

	openTxn.transaction.StorageProofs = append(openTxn.transaction.StorageProofs, sp)
	return nil
}

// AddArbitraryData implements the core.Wallet interface.
func (w *Wallet) AddArbitraryData(id string, arb string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	openTxn, exists := w.transactions[id]
	if !exists {
		return errors.New("no transaction found for given id")
	}

	openTxn.transaction.ArbitraryData = append(openTxn.transaction.ArbitraryData, arb)
	return nil
}

// SignTransaction implements the core.Wallet interface.
func (w *Wallet) SignTransaction(id string, wholeTransaction bool) (txn consensus.Transaction, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Fetch the transaction.
	openTxn, exists := w.transactions[id]
	if !exists {
		err = errors.New("no transaction found for given id")
		return
	}
	txn = *openTxn.transaction

	// Get the coveredfields struct.
	var coveredFields consensus.CoveredFields
	if wholeTransaction {
		coveredFields = consensus.CoveredFields{WholeTransaction: true}
	} else {
		for i := range txn.MinerFees {
			coveredFields.MinerFees = append(coveredFields.MinerFees, uint64(i))
		}
		for i := range txn.Inputs {
			coveredFields.Inputs = append(coveredFields.Inputs, uint64(i))
		}
		for i := range txn.Outputs {
			coveredFields.Outputs = append(coveredFields.Outputs, uint64(i))
		}
		for i := range txn.FileContracts {
			coveredFields.FileContracts = append(coveredFields.FileContracts, uint64(i))
		}
		for i := range txn.StorageProofs {
			coveredFields.StorageProofs = append(coveredFields.StorageProofs, uint64(i))
		}
		// TODO: Siafund stuff here.
		for i := range txn.ArbitraryData {
			coveredFields.ArbitraryData = append(coveredFields.ArbitraryData, uint64(i))
		}
		for i := range txn.Signatures {
			coveredFields.Signatures = append(coveredFields.Signatures, uint64(i))
		}
	}

	// For each input in the transaction that we added, provide a signature.
	for _, inputIndex := range openTxn.inputs {
		input := txn.Inputs[inputIndex]
		sig := consensus.TransactionSignature{
			InputID:        input.OutputID,
			CoveredFields:  coveredFields,
			PublicKeyIndex: 0,
		}
		txn.Signatures = append(txn.Signatures, sig)

		// Hash the transaction according to the covered fields.
		coinAddress := input.SpendConditions.CoinAddress()
		sigIndex := len(txn.Signatures) - 1
		secKey := w.keys[coinAddress].secretKey
		sigHash := txn.SigHash(sigIndex)

		// Get the signature.
		var encodedSig crypto.Signature
		encodedSig, err = crypto.SignBytes(sigHash[:], secKey)
		if err != nil {
			return
		}
		copy(txn.Signatures[sigIndex].Signature, encodedSig[:])
	}

	// Delete the open transaction.
	delete(w.transactions, id)

	return
}
