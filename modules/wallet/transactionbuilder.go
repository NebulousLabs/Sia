package wallet

import (
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

var (
	ErrInvalidID = errors.New("no transaction of given id found")
)

// openTransaction is a type that the wallet uses to track a transaction while
// building it out as a custom transaction.
type openTransaction struct {
	// parents is a list of all unconfirmed dependencies to 'transaction'.
	// 'transaction' is the work-in-progress transaction.
	parents     []types.Transaction
	transaction types.Transaction

	// inputs lists by index all of the inputs that were added to the
	// transaction using 'FundTransaction'. These are the inputs that will be
	// signed when 'SignTransaction' is called.
	inputs []int
}

// RegisterTransaction takes a transaction and its parents returns an id that
// can be used to modify the transaction. The most typical call is
// 'RegisterTransaction(types.Transaction{}, nil)', which registers a new
// transaction that doesn't have any parents. The id that gets returned is not
// a types.TransactionID, it is an int and is only useful within the
// transaction builder.
func (w *Wallet) RegisterTransaction(t types.Transaction, parents []types.Transaction) (int, error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	id := w.registryCounter
	w.registryCounter++
	w.transactionRegistry[id] = &openTransaction{
		parents:     parents,
		transaction: t,
	}
	return id, nil
}

// FundTransaction will create a transaction with a siacoin output containing a
// value of exactly 'amount' - this prevents any refunds from appearing in the
// primary transaction, but adds some number (usually one, but can be more or
// less) of parent transactions. The parent transactions are signed
// immediately, but the child transaction will not be signed until
// 'SignTransaction' is called.
//
// TODO: Make sure that the addressing leverages the wallet's age controls to
// prevent stalled money from becoming a problem.
func (w *Wallet) FundTransaction(id int, amount types.Currency) error {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	// Get the transaction that was originally meant to be funded.
	openTxn, exists := w.transactionRegistry[id]
	if !exists {
		return ErrInvalidID
	}

	// Create a parent transaction and supply it with enough inputs to cover
	// 'amount'.
	parentTxn := types.Transaction{}
	fundingOutputs, fundingTotal, err := w.findOutputs(amount)
	if err != nil {
		return err
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
			return err
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
			return err
		}
		parentTxn.TransactionSignatures[sigIndex].Signature = encodedSig[:]
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

	// Add the exact output.
	newInput := types.SiacoinInput{
		ParentID:         parentTxn.SiacoinOutputID(0),
		UnlockConditions: parentSpendConds,
	}
	openTxn.parents = append(openTxn.parents, parentTxn)
	openTxn.inputs = append(openTxn.inputs, len(openTxn.transaction.SiacoinInputs))
	openTxn.transaction.SiacoinInputs = append(openTxn.transaction.SiacoinInputs, newInput)
	return nil
}

// AddMinerFee adds a single miner fee of value 'fee' to a transaction
// specified by the registration id. The index of the fee within the
// transaction is returned.
func (w *Wallet) AddMinerFee(id int, fee types.Currency) (uint64, error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	openTxn, exists := w.transactionRegistry[id]
	if !exists {
		return 0, ErrInvalidID
	}
	openTxn.transaction.MinerFees = append(openTxn.transaction.MinerFees, fee)
	return uint64(len(openTxn.transaction.MinerFees) - 1), nil
}

// AddSiacoinInput adds a siacoin input to a transaction, specified by the
// registration id.  When 'SignTransaction' gets called, this input will be
// left unsigned.  The index of the siacoin input within the transaction is
// returned.
func (w *Wallet) AddSiacoinInput(id int, input types.SiacoinInput) (uint64, error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	openTxn, exists := w.transactionRegistry[id]
	if !exists {
		return 0, ErrInvalidID
	}
	openTxn.transaction.SiacoinInputs = append(openTxn.transaction.SiacoinInputs, input)
	return uint64(len(openTxn.transaction.SiacoinInputs) - 1), nil
}

// AddSiacoinOutput adds an output to a transaction, specified by id. The index
// of the siacoin output within the transaction is returned.
func (w *Wallet) AddSiacoinOutput(id int, output types.SiacoinOutput) (uint64, error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	openTxn, exists := w.transactionRegistry[id]
	if !exists {
		return 0, ErrInvalidID
	}
	openTxn.transaction.SiacoinOutputs = append(openTxn.transaction.SiacoinOutputs, output)
	return uint64(len(openTxn.transaction.SiacoinOutputs) - 1), nil
}

// AddFileContract adds a file contract to a transaction, specified by id.  The
// index of the file contract within the transaction is returned.
func (w *Wallet) AddFileContract(id int, fc types.FileContract) (uint64, error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	openTxn, exists := w.transactionRegistry[id]
	if !exists {
		return 0, ErrInvalidID
	}
	openTxn.transaction.FileContracts = append(openTxn.transaction.FileContracts, fc)
	return uint64(len(openTxn.transaction.FileContracts) - 1), nil
}

// AddFileContract adds a file contract to a transaction, specified by id.  The
// index of the file contract within the transaction is returned.
func (w *Wallet) AddFileContractRevision(id int, fcr types.FileContractRevision) (uint64, error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	openTxn, exists := w.transactionRegistry[id]
	if !exists {
		return 0, ErrInvalidID
	}
	openTxn.transaction.FileContractRevisions = append(openTxn.transaction.FileContractRevisions, fcr)
	return uint64(len(openTxn.transaction.FileContractRevisions) - 1), nil
}

// AddStorageProof adds a storage proof to a transaction, specified by id.  The
// index of the storage proof within the transaction is returned.
func (w *Wallet) AddStorageProof(id int, sp types.StorageProof) (uint64, error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	openTxn, exists := w.transactionRegistry[id]
	if !exists {
		return 0, ErrInvalidID
	}
	openTxn.transaction.StorageProofs = append(openTxn.transaction.StorageProofs, sp)
	return uint64(len(openTxn.transaction.StorageProofs) - 1), nil
}

// AddSiafundInput adds a siacoin input to the transaction, specified by id.
// When 'SignTransaction' gets called, this input will be left unsigned. The
// index of the siafund input within the transaction is returned.
func (w *Wallet) AddSiafundInput(id int, input types.SiafundInput) (uint64, error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	openTxn, exists := w.transactionRegistry[id]
	if !exists {
		return 0, ErrInvalidID
	}
	openTxn.transaction.SiafundInputs = append(openTxn.transaction.SiafundInputs, input)
	return uint64(len(openTxn.transaction.SiafundInputs) - 1), nil
}

// AddSiafundOutput adds an output to a transaction, specified by registration
// id. The index of the siafund output within the transaction is returned.
func (w *Wallet) AddSiafundOutput(id int, output types.SiafundOutput) (uint64, error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	openTxn, exists := w.transactionRegistry[id]
	if !exists {
		return 0, ErrInvalidID
	}
	openTxn.transaction.SiafundOutputs = append(openTxn.transaction.SiafundOutputs, output)
	return uint64(len(openTxn.transaction.SiafundOutputs) - 1), nil
}

// AddArbitraryData adds a byte slice to the arbitrary data section of the
// transaction. The index of the arbitrary data within the transaction is
// returned.
func (w *Wallet) AddArbitraryData(id int, arb []byte) (uint64, error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	openTxn, exists := w.transactionRegistry[id]
	if !exists {
		return 0, ErrInvalidID
	}

	openTxn.transaction.ArbitraryData = append(openTxn.transaction.ArbitraryData, arb)
	return uint64(len(openTxn.transaction.ArbitraryData) - 1), nil
}

// AddTransactionSignature adds a signature to the transaction, the signature
// should already be valid, and shouldn't sign any of the inputs that were
// added by calling 'FundTransaction'. The updated transaction and the index of
// the new signature are returned.
func (w *Wallet) AddTransactionSignature(id int, sig types.TransactionSignature) (uint64, error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	openTxn, exists := w.transactionRegistry[id]
	if !exists {
		return 0, ErrInvalidID
	}
	openTxn.transaction.TransactionSignatures = append(openTxn.transaction.TransactionSignatures, sig)
	return uint64(len(openTxn.transaction.TransactionSignatures) - 1), nil
}

// SignTransaction will sign and delete a transaction, specified by
// registration id. If the whole transaction flag is set to true, then the
// covered fields object in each of the transaction signatures will have the
// whole transaction field set. Otherwise, the flag will not be set but the
// signature will cover all known fields in the transaction (see an
// implementation for more clarity). After signing, a transaction set will be
// returned that contains all parents followed by the transaction. The
// transaction is then deleted from the builder registry.
func (w *Wallet) SignTransaction(id int, wholeTransaction bool) ([]types.Transaction, error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	// Fetch the transaction.
	openTxn, exists := w.transactionRegistry[id]
	if !exists {
		return nil, ErrInvalidID
	}
	txn := openTxn.transaction

	// Create the coveredfields struct.
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
		for i := range txn.FileContractRevisions {
			coveredFields.FileContractRevisions = append(coveredFields.FileContractRevisions, uint64(i))
		}
		for i := range txn.StorageProofs {
			coveredFields.StorageProofs = append(coveredFields.StorageProofs, uint64(i))
		}
		for i := range txn.SiafundInputs {
			coveredFields.SiafundInputs = append(coveredFields.SiafundInputs, uint64(i))
		}
		for i := range txn.SiafundOutputs {
			coveredFields.SiafundOutputs = append(coveredFields.SiafundOutputs, uint64(i))
		}
		for i := range txn.ArbitraryData {
			coveredFields.ArbitraryData = append(coveredFields.ArbitraryData, uint64(i))
		}
	}
	// TransactionSignatures don't get covered by the 'WholeTransaction' flag,
	// and must be covered manually.
	for i := range txn.TransactionSignatures {
		coveredFields.TransactionSignatures = append(coveredFields.TransactionSignatures, uint64(i))
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
		encodedSig, err := crypto.SignHash(sigHash, secKey)
		if err != nil {
			return nil, err
		}
		txn.TransactionSignatures[sigIndex].Signature = encodedSig[:]
	}

	// Get the transaction set and delete the transaction from the registry.
	txnSet := append(openTxn.parents, txn)
	delete(w.transactionRegistry, id)

	return txnSet, nil
}

// ViewTransaction returns a transaction-in-progress along with all of its
// parents, specified by id. An error is returned if the id is invalid.  Note
// that ids become invalid for a transaction after 'SignTransaction' has been
// called because the transaction gets deleted.
func (w *Wallet) ViewTransaction(id int) (types.Transaction, []types.Transaction, error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	openTxn, exists := w.transactionRegistry[id]
	if !exists {
		return types.Transaction{}, nil, ErrInvalidID
	}
	return openTxn.transaction, openTxn.parents, nil
}
