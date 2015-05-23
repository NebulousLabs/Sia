package wallet

import (
	"errors"
	"strconv"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

var (
	ErrInvalidID = errors.New("no transaction of given id found")
)

// openTransaction is a type that the wallet uses to track a transaction as it
// adds inputs and other features. `inputs` is a list of inputs (their indicies
// in the transaction) that the wallet has added personally, so that the inputs
// can be signed when SignTransaction() is called.
type openTransaction struct {
	transaction *types.Transaction
	inputs      []int
}

// RegisterTransaction starts with a transaction as input and adds that
// transaction to the list of open transactions, returning an id. That id can
// then be used to modify and sign the transaction. An empty transaction is
// legal input.
func (w *Wallet) RegisterTransaction(t types.Transaction) (id string, err error) {
	counter := w.mu.Lock()
	defer w.mu.Unlock(counter)

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
func (w *Wallet) FundTransaction(id string, amount types.Currency) (t types.Transaction, err error) {
	counter := w.mu.Lock()
	defer w.mu.Unlock(counter)

	// Create a parent transaction and supply it with enough inputs to cover
	// 'amount'.
	parentTxn := types.Transaction{}
	fundingOutputs, fundingTotal, err := w.findOutputs(amount)
	if err != nil {
		return
	}
	for _, output := range fundingOutputs {
		output.age = w.age
		key := w.keys[output.output.UnlockHash]
		newInput := types.SiacoinInput{
			ParentID:         output.id,
			UnlockConditions: key.unlockConditions,
		}
		parentTxn.SiacoinInputs = append(parentTxn.SiacoinInputs, newInput)
	}

	// Create and add the output that will be used to fund the standard
	// transaction.
	parentDest, parentSpendConds, err := w.coinAddress(false) // false indicates that the address should not be visible to the user
	exactOutput := types.SiacoinOutput{
		Value:      amount,
		UnlockHash: parentDest,
	}
	parentTxn.SiacoinOutputs = append(parentTxn.SiacoinOutputs, exactOutput)

	// Create a refund output if needed.
	if amount.Cmp(fundingTotal) != 0 {
		var refundDest types.UnlockHash
		refundDest, _, err = w.coinAddress(false) // false indicates that the address should not be visible to the user
		if err != nil {
			return
		}
		refundOutput := types.SiacoinOutput{
			Value:      fundingTotal.Sub(amount),
			UnlockHash: refundDest,
		}
		parentTxn.SiacoinOutputs = append(parentTxn.SiacoinOutputs, refundOutput)
	}

	// Sign all of the inputs to the parent trancstion.
	coveredFields := types.CoveredFields{WholeTransaction: true}
	for _, input := range parentTxn.SiacoinInputs {
		sig := types.TransactionSignature{
			ParentID:       crypto.Hash(input.ParentID),
			CoveredFields:  coveredFields,
			PublicKeyIndex: 0,
		}
		parentTxn.TransactionSignatures = append(parentTxn.TransactionSignatures, sig)

		// Hash the transaction according to the covered fields.
		coinAddress := input.UnlockConditions.UnlockHash()
		sigIndex := len(parentTxn.TransactionSignatures) - 1
		secKey := w.keys[coinAddress].secretKey
		sigHash := parentTxn.SigHash(sigIndex)

		// Get the signature.
		var encodedSig crypto.Signature
		encodedSig, err = crypto.SignHash(sigHash, secKey)
		if err != nil {
			return
		}
		parentTxn.TransactionSignatures[sigIndex].Signature = types.Signature(encodedSig[:])
	}

	// Add the exact output to the wallet's knowledgebase before releasing the
	// lock, to prevent the wallet from using the exact output elsewhere.
	key := w.keys[parentSpendConds.UnlockHash()]
	key.outputs[parentTxn.SiacoinOutputID(0)] = &knownOutput{
		id:     parentTxn.SiacoinOutputID(0),
		output: exactOutput,

		spendable: true,
		age:       w.age,
	}

	// Send the transaction to the transaction pool.
	err = w.tpool.AcceptTransaction(parentTxn)
	if err != nil {
		return
	}

	// Get the transaction that was originally meant to be funded.
	openTxn, exists := w.transactions[id]
	if !exists {
		err = ErrInvalidID
		return
	}
	txn := openTxn.transaction

	// Add the exact output.
	newInput := types.SiacoinInput{
		ParentID:         parentTxn.SiacoinOutputID(0),
		UnlockConditions: parentSpendConds,
	}
	openTxn.inputs = append(openTxn.inputs, len(txn.SiacoinInputs))
	txn.SiacoinInputs = append(txn.SiacoinInputs, newInput)
	t = *txn
	return
}

// AddSiacoinInput will add a siacoin input to the transaction, returning the
// index of the input within the transaction and the transaction itself. When
// 'SignTransaction' is called, this input will not be signed.
func (w *Wallet) AddSiacoinInput(id string, input types.SiacoinInput) (t types.Transaction, inputIndex uint64, err error) {
	counter := w.mu.Lock()
	defer w.mu.Unlock(counter)

	openTxn, exists := w.transactions[id]
	if !exists {
		err = ErrInvalidID
		return
	}

	openTxn.transaction.SiacoinInputs = append(openTxn.transaction.SiacoinInputs, input)
	t = *openTxn.transaction
	inputIndex = uint64(len(t.SiacoinInputs) - 1)
	return
}

// AddMinerFee will add a miner fee to the transaction, but will not add any
// inputs. The transaction and the index of the new miner fee within the
// transaction are returned.
func (w *Wallet) AddMinerFee(id string, fee types.Currency) (t types.Transaction, feeIndex uint64, err error) {
	counter := w.mu.Lock()
	defer w.mu.Unlock(counter)

	openTxn, exists := w.transactions[id]
	if !exists {
		err = ErrInvalidID
		return
	}

	openTxn.transaction.MinerFees = append(openTxn.transaction.MinerFees, fee)
	t = *openTxn.transaction
	feeIndex = uint64(len(t.MinerFees) - 1)
	return
}

// AddOutput adds an output to the transaction, but will not add any inputs.
// AddOutput returns the transaction and the index of the new output within the
// transaction.
func (w *Wallet) AddOutput(id string, output types.SiacoinOutput) (t types.Transaction, outputIndex uint64, err error) {
	counter := w.mu.Lock()
	defer w.mu.Unlock(counter)

	openTxn, exists := w.transactions[id]
	if !exists {
		err = ErrInvalidID
		return
	}

	openTxn.transaction.SiacoinOutputs = append(openTxn.transaction.SiacoinOutputs, output)
	t = *openTxn.transaction
	outputIndex = uint64(len(t.SiacoinOutputs) - 1)
	return
}

// AddFileContract adds a file contract to the transaction, returning a copy of
// the transaction and the index of the new file contract within the
// transaction.
func (w *Wallet) AddFileContract(id string, fc types.FileContract) (t types.Transaction, fcIndex uint64, err error) {
	counter := w.mu.Lock()
	defer w.mu.Unlock(counter)

	openTxn, exists := w.transactions[id]
	if !exists {
		err = ErrInvalidID
		return
	}

	openTxn.transaction.FileContracts = append(openTxn.transaction.FileContracts, fc)
	t = *openTxn.transaction
	fcIndex = uint64(len(t.FileContracts) - 1)
	return
}

// AddStorageProof adds a storage proof to the transaction, returning a copy of
// the transaction and the index of the new storage proof within the
// transaction.
func (w *Wallet) AddStorageProof(id string, sp types.StorageProof) (t types.Transaction, spIndex uint64, err error) {
	counter := w.mu.Lock()
	defer w.mu.Unlock(counter)

	openTxn, exists := w.transactions[id]
	if !exists {
		err = ErrInvalidID
		return
	}

	openTxn.transaction.StorageProofs = append(openTxn.transaction.StorageProofs, sp)
	t = *openTxn.transaction
	spIndex = uint64(len(t.StorageProofs) - 1)
	return
}

// AddArbitraryData adds arbitrary data to the transaction, returning a copy of
// the transaction and the index of the new data within the transaction.
func (w *Wallet) AddArbitraryData(id string, arb string) (t types.Transaction, adIndex uint64, err error) {
	counter := w.mu.Lock()
	defer w.mu.Unlock(counter)

	openTxn, exists := w.transactions[id]
	if !exists {
		err = ErrInvalidID
		return
	}

	openTxn.transaction.ArbitraryData = append(openTxn.transaction.ArbitraryData, arb)
	t = *openTxn.transaction
	adIndex = uint64(len(t.ArbitraryData) - 1)
	return
}

// SignTransaction signs the transaction, then deletes the transaction from the
// wallet's internal memory, then returns the transaction.
func (w *Wallet) SignTransaction(id string, wholeTransaction bool) (txn types.Transaction, err error) {
	counter := w.mu.Lock()
	defer w.mu.Unlock(counter)

	// Fetch the transaction.
	openTxn, exists := w.transactions[id]
	if !exists {
		err = ErrInvalidID
		return
	}
	txn = *openTxn.transaction

	// Get the coveredfields struct.
	var coveredFields types.CoveredFields
	if wholeTransaction {
		coveredFields = types.CoveredFields{WholeTransaction: true}
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
		for i := range txn.ArbitraryData {
			coveredFields.ArbitraryData = append(coveredFields.ArbitraryData, uint64(i))
		}
		for i := range txn.TransactionSignatures {
			coveredFields.TransactionSignatures = append(coveredFields.TransactionSignatures, uint64(i))
		}
	}

	// For each input in the transaction that we added, provide a signature.
	for _, inputIndex := range openTxn.inputs {
		input := txn.SiacoinInputs[inputIndex]
		sig := types.TransactionSignature{
			ParentID:       crypto.Hash(input.ParentID),
			CoveredFields:  coveredFields,
			PublicKeyIndex: 0,
		}
		txn.TransactionSignatures = append(txn.TransactionSignatures, sig)

		// Hash the transaction according to the covered fields.
		coinAddress := input.UnlockConditions.UnlockHash()
		sigIndex := len(txn.TransactionSignatures) - 1
		secKey := w.keys[coinAddress].secretKey
		sigHash := txn.SigHash(sigIndex)

		// Get the signature.
		var encodedSig crypto.Signature
		encodedSig, err = crypto.SignHash(sigHash, secKey)
		if err != nil {
			return
		}
		txn.TransactionSignatures[sigIndex].Signature = types.Signature(encodedSig[:])
	}

	// Delete the open transaction.
	delete(w.transactions, id)

	return
}

// AddSignature adds a signature to the transaction, presumably signing one of
// the inputs that 'SignTransaction' will not sign automatically. This can be
// useful for dealing with multiparty signatures, or for staged negotiations
// which involve sending the transaction first and the signature later.
func (w *Wallet) AddSignature(id string, sig types.TransactionSignature) (t types.Transaction, sigIndex uint64, err error) {
	counter := w.mu.Lock()
	defer w.mu.Unlock(counter)

	openTxn, exists := w.transactions[id]
	if !exists {
		err = ErrInvalidID
		return
	}

	openTxn.transaction.TransactionSignatures = append(openTxn.transaction.TransactionSignatures, sig)
	t = *openTxn.transaction
	sigIndex = uint64(len(t.TransactionSignatures) - 1)
	return
}
