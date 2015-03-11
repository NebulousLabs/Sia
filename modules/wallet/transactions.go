package wallet

import (
	"errors"
	"strconv"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
)

var (
	ErrInvalidID = errors.New("no transaction of given id found")
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
	counter := w.mu.Lock("wallet RegisterTransaction")
	defer w.mu.Unlock("wallet RegisterTransaction", counter)

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
func (w *Wallet) FundTransaction(id string, amount consensus.Currency) (t consensus.Transaction, err error) {
	counter := w.mu.Lock("wallet FundTransaction")
	defer w.mu.Unlock("wallet FundTransaction", counter)

	// Create a parent transaction and supply it with enough inputs to cover
	// 'amount'.
	parentTxn := consensus.Transaction{}
	fundingOutputs, fundingTotal, err := w.findOutputs(amount)
	if err != nil {
		return
	}
	for _, output := range fundingOutputs {
		key := w.keys[output.output.UnlockHash]
		newInput := consensus.SiacoinInput{
			ParentID:         output.id,
			UnlockConditions: key.unlockConditions,
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
		refundDest, _, err = w.coinAddress()
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
		err = ErrInvalidID
		return
	}
	txn := openTxn.transaction

	// Add the exact output.
	newInput := consensus.SiacoinInput{
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
func (w *Wallet) AddSiacoinInput(id string, input consensus.SiacoinInput) (t consensus.Transaction, inputIndex uint64, err error) {
	counter := w.mu.Lock("wallet AddSiacoinInput")
	defer w.mu.Unlock("wallet AddSiacoinInput", counter)

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
func (w *Wallet) AddMinerFee(id string, fee consensus.Currency) (t consensus.Transaction, feeIndex uint64, err error) {
	counter := w.mu.Lock("wallet AddMinerFee")
	defer w.mu.Unlock("wallet AddMinerFee", counter)

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
func (w *Wallet) AddOutput(id string, output consensus.SiacoinOutput) (t consensus.Transaction, outputIndex uint64, err error) {
	counter := w.mu.Lock("wallet AddOutput")
	defer w.mu.Unlock("wallet AddOutput", counter)

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
func (w *Wallet) AddFileContract(id string, fc consensus.FileContract) (t consensus.Transaction, fcIndex uint64, err error) {
	counter := w.mu.Lock("wallet AddFileContract")
	defer w.mu.Unlock("wallet AddFileContract", counter)

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
func (w *Wallet) AddStorageProof(id string, sp consensus.StorageProof) (t consensus.Transaction, spIndex uint64, err error) {
	counter := w.mu.Lock("wallet AddStorageProof")
	defer w.mu.Unlock("wallet AddStorageProof", counter)

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
func (w *Wallet) AddArbitraryData(id string, arb string) (t consensus.Transaction, adIndex uint64, err error) {
	counter := w.mu.Lock("wallet AddArbitraryData")
	defer w.mu.Unlock("wallet AddArbitraryData", counter)

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
func (w *Wallet) SignTransaction(id string, wholeTransaction bool) (txn consensus.Transaction, err error) {
	counter := w.mu.Lock("wallet SignTransaction")
	defer w.mu.Unlock("wallet SignTransaction", counter)

	// Fetch the transaction.
	openTxn, exists := w.transactions[id]
	if !exists {
		err = ErrInvalidID
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

// AddSignature adds a signature to the transaction, presumably signing one of
// the inputs that 'SignTransaction' will not sign automatically. This can be
// useful for dealing with multiparty signatures, or for staged negotiations
// which involve sending the transaction first and the signature later.
func (w *Wallet) AddSignature(id string, sig consensus.TransactionSignature) (t consensus.Transaction, sigIndex uint64, err error) {
	counter := w.mu.Lock("wallet AddSignature")
	defer w.mu.Unlock("wallet AddSignature", counter)

	openTxn, exists := w.transactions[id]
	if !exists {
		err = ErrInvalidID
		return
	}

	openTxn.transaction.Signatures = append(openTxn.transaction.Signatures, sig)
	t = *openTxn.transaction
	sigIndex = uint64(len(t.Signatures) - 1)
	return
}
