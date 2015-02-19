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

// FundTransaction adds siacoins to a transaction that the wallet knows how to
// spend. The exact amount of coins are always added, and this is achieved by
// creating two transactions. The first transaciton, the parent, spends a set
// of outputs that add up to at least the desired amount, and then creates a
// single output of the exact amount and a second refund output.
func (w *Wallet) FundTransaction(id string, amount consensus.Currency) (err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Create a parent transaction and supply it with enough inputs to cover
	// 'amount'.
	parentTxn := consensus.Transaction{}
	fundingOutputs, fundingTotal, err := w.findOutputs(amount)
	if err != nil {
		return err
	}
	for _, output := range fundingOutputs {
		key := w.keys[output.output.UnlockHash]
		newInput := consensus.SiacoinInput{
			ParentID:         output.id,
			UnlockConditions: key.spendConditions,
		}
		parentTxn.SiacoinInputs = append(parentTxn.SiacoinInputs, newInput)
	}

	// Create and add the output that will be used to fund the standard
	// transaction.
	parentDest, parentSpendConds, err := w.coinAddress()
	exactOutput := consensus.SiacoinOutput{
		Value:      amount,
		UnlockHash: parentDest,
	}
	parentTxn.SiacoinOutputs = append(parentTxn.SiacoinOutputs, exactOutput)

	// Create a refund output if needed.
	if amount.Cmp(fundingTotal) != 0 {
		var refundDest consensus.UnlockHash
		refundDest, _, err = w.CoinAddress()
		if err != nil {
			return
		}
		refundOutput := consensus.SiacoinOutput{
			Value:      fundingTotal.Sub(amount),
			UnlockHash: refundDest,
		}
		parentTxn.SiacoinOutputs = append(parentTxn.SiacoinOutputs, refundOutput)
	}

	// Sign all of the inputs to the parent trancstion.
	coveredFields := consensus.CoveredFields{WholeTransaction: true}
	for _, input := range parentTxn.SiacoinInputs {
		sig := consensus.TransactionSignature{
			ParentID:       crypto.Hash(input.ParentID),
			CoveredFields:  coveredFields,
			PublicKeyIndex: 0,
		}
		parentTxn.Signatures = append(parentTxn.Signatures, sig)

		// Hash the transaction according to the covered fields.
		coinAddress := input.UnlockConditions.UnlockHash()
		sigIndex := len(parentTxn.Signatures) - 1
		secKey := w.keys[coinAddress].secretKey
		sigHash := parentTxn.SigHash(sigIndex)

		// Get the signature.
		var encodedSig crypto.Signature
		encodedSig, err = crypto.SignHash(sigHash, secKey)
		if err != nil {
			return
		}
		parentTxn.Signatures[sigIndex].Signature = consensus.Signature(encodedSig[:])
	}

	// Add the exact output to the wallet's knowledgebase before releasing the
	// lock, to prevent the wallet from using the exact output elsewhere.
	key := w.keys[parentSpendConds.UnlockHash()]
	key.outputs[parentTxn.SiacoinOutputID(0)] = &knownOutput{
		id:     parentTxn.SiacoinOutputID(0),
		output: exactOutput,
		age:    w.age,
	}

	// Send the transaction to the transaction pool.
	err = w.tpool.AcceptTransaction(parentTxn)
	if err != nil {
		return
	}

	// Get the transaction that was originally meant to be funded.
	openTxn, exists := w.transactions[id]
	if !exists {
		return errors.New("no transaction of given id found")
	}
	txn := openTxn.transaction

	// Add the exact output.
	newInput := consensus.SiacoinInput{
		ParentID:         parentTxn.SiacoinOutputID(0),
		UnlockConditions: parentSpendConds,
	}
	openTxn.inputs = append(openTxn.inputs, len(txn.SiacoinInputs))
	txn.SiacoinInputs = append(txn.SiacoinInputs, newInput)
	return
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
func (w *Wallet) AddOutput(id string, output consensus.SiacoinOutput) (index uint64, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	openTxn, exists := w.transactions[id]
	if !exists {
		err = errors.New("no transaction found for given id")
		return
	}

	openTxn.transaction.SiacoinOutputs = append(openTxn.transaction.SiacoinOutputs, output)
	index = uint64(len(openTxn.transaction.SiacoinOutputs) - 1)
	return
}

// AddFileContract adds a file contract to the transaction.
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
		for i := range txn.SiacoinInputs {
			coveredFields.SiacoinInputs = append(coveredFields.SiacoinInputs, uint64(i))
		}
		for i := range txn.SiacoinOutputs {
			coveredFields.SiacoinOutputs = append(coveredFields.SiacoinOutputs, uint64(i))
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
		input := txn.SiacoinInputs[inputIndex]
		sig := consensus.TransactionSignature{
			ParentID:       crypto.Hash(input.ParentID),
			CoveredFields:  coveredFields,
			PublicKeyIndex: 0,
		}
		txn.Signatures = append(txn.Signatures, sig)

		// Hash the transaction according to the covered fields.
		coinAddress := input.UnlockConditions.UnlockHash()
		sigIndex := len(txn.Signatures) - 1
		secKey := w.keys[coinAddress].secretKey
		sigHash := txn.SigHash(sigIndex)

		// Get the signature.
		var encodedSig crypto.Signature
		encodedSig, err = crypto.SignHash(sigHash, secKey)
		if err != nil {
			return
		}
		txn.Signatures[sigIndex].Signature = consensus.Signature(encodedSig[:])
	}

	// Delete the open transaction.
	delete(w.transactions, id)

	return
}
