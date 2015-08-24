package wallet

import (
	"bytes"
	"sort"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

type transactionBuilder struct {
	parents       []types.Transaction
	transaction   types.Transaction
	siacoinInputs []int
	siafundInputs []int

	wallet *Wallet
}

// addSignatures will sign a transaction using a spendable key, with support
// for multisig spendable keys. Because of the restricted input, the function
// is compatible with both siacoin inputs and siafund inputs.
func addSignatures(txn *types.Transaction, cf types.CoveredFields, uc types.UnlockConditions, parentID crypto.Hash, key spendableKey) error {
	usedIndices := make(map[int]struct{})
	for i := range key.SecretKeys {
		found := false
		keyIndex := 0
		pubKey := key.SecretKeys[i].PublicKey()
		for i, siaPublicKey := range uc.PublicKeys {
			_, exists := usedIndices[i]
			if !exists && bytes.Compare(pubKey[:], siaPublicKey.Key) == 0 {
				found = true
				keyIndex = i
				break
			}
		}
		if !found && build.DEBUG {
			panic("transaction builder cannot sign an input that it added")
		}
		usedIndices[keyIndex] = struct{}{}

		// Create the unsigned transaction signature.
		sig := types.TransactionSignature{
			ParentID:       parentID,
			CoveredFields:  cf,
			PublicKeyIndex: uint64(keyIndex),
		}
		txn.TransactionSignatures = append(txn.TransactionSignatures, sig)

		// Get the signature.
		sigIndex := len(txn.TransactionSignatures) - 1
		sigHash := txn.SigHash(sigIndex)
		encodedSig, err := crypto.SignHash(sigHash, key.SecretKeys[i])
		if err != nil {
			return err
		}
		txn.TransactionSignatures[sigIndex].Signature = encodedSig[:]
	}
	return nil
}

// RegisterTransaction takes a transaction and its parents and returns a
// TransactionBuilder which can be used to expand the transaction. The most
// typical call is 'RegisterTransaction(types.Transaction{}, nil)', which
// registers a new transaction without parents.
func (w *Wallet) RegisterTransaction(t types.Transaction, parents []types.Transaction) modules.TransactionBuilder {
	return &transactionBuilder{
		parents:     parents,
		transaction: t,

		wallet: w,
	}
}

// StartTransaction is a convenience function that calls
// RegisterTransaction(types.Transaction{}, nil).
func (w *Wallet) StartTransaction() modules.TransactionBuilder {
	return w.RegisterTransaction(types.Transaction{}, nil)
}

// FundSiacoins will add a siacoin input of exaclty 'amount' to the
// transaction. A parent transaction may be needed to achieve an input with the
// correct value. The siacoin input will not be signed until 'Sign' is called
// on the transaction builder.
func (tb *transactionBuilder) FundSiacoins(amount types.Currency) error {
	lockID := tb.wallet.mu.Lock()
	defer tb.wallet.mu.Unlock(lockID)

	// Collect a value-sorted set of siacoin outputs.
	var so sortedOutputs
	for scoid, sco := range tb.wallet.siacoinOutputs {
		so.ids = append(so.ids, scoid)
		so.outputs = append(so.outputs, sco)
	}
	sort.Sort(so)

	// Create and fund a parent transaction that will add the correct amount of
	// siacoins to the transaction.
	var fund types.Currency
	// potentialFund tracks the balance of the wallet including outputs that
	// have been spent in other unconfirmed transactions recently. This is to
	// provide the user with a more useful error message in the event that they
	// are overspending.
	var potentialFund types.Currency
	parentTxn := types.Transaction{}
	var spentScoids []types.SiacoinOutputID
	for i := range so.ids {
		scoid := so.ids[i]
		sco := so.outputs[i]
		// Check that this output has not recently been spent by the wallet.
		spendHeight := tb.wallet.spentOutputs[types.OutputID(scoid)]
		// Prevent an underflow error.
		allowedHeight := tb.wallet.consensusSetHeight - RespendTimeout
		if tb.wallet.consensusSetHeight < RespendTimeout {
			allowedHeight = 0
		}
		if spendHeight > allowedHeight {
			potentialFund = potentialFund.Add(sco.Value)
			continue
		}
		outputUnlockConditions := tb.wallet.keys[sco.UnlockHash].UnlockConditions
		if tb.wallet.consensusSetHeight < outputUnlockConditions.Timelock {
			continue
		}

		// Add a siacoin input for this output.
		sci := types.SiacoinInput{
			ParentID:         scoid,
			UnlockConditions: outputUnlockConditions,
		}
		parentTxn.SiacoinInputs = append(parentTxn.SiacoinInputs, sci)
		spentScoids = append(spentScoids, scoid)

		// Add the output to the total fund
		fund = fund.Add(sco.Value)
		potentialFund = potentialFund.Add(sco.Value)
		if fund.Cmp(amount) >= 0 {
			break
		}
	}
	if potentialFund.Cmp(amount) > 0 && fund.Cmp(amount) < 0 {
		return modules.ErrPotentialDoubleSpend
	}
	if fund.Cmp(amount) < 0 {
		return modules.ErrLowBalance
	}

	// Create and add the output that will be used to fund the standard
	// transaction.
	parentUnlockConditions, err := tb.wallet.nextPrimarySeedAddress()
	if err != nil {
		return err
	}
	exactOutput := types.SiacoinOutput{
		Value:      amount,
		UnlockHash: parentUnlockConditions.UnlockHash(),
	}
	parentTxn.SiacoinOutputs = append(parentTxn.SiacoinOutputs, exactOutput)

	// Create a refund output if needed.
	if amount.Cmp(fund) != 0 {
		refundUnlockConditions, err := tb.wallet.nextPrimarySeedAddress()
		if err != nil {
			return err
		}
		refundOutput := types.SiacoinOutput{
			Value:      fund.Sub(amount),
			UnlockHash: refundUnlockConditions.UnlockHash(),
		}
		parentTxn.SiacoinOutputs = append(parentTxn.SiacoinOutputs, refundOutput)
	}

	// Sign all of the inputs to the parent trancstion. This is done at the end
	// so that the outputs aren't marked as spent in the event of an error.
	for _, sci := range parentTxn.SiacoinInputs {
		err := addSignatures(&parentTxn, types.FullCoveredFields, sci.UnlockConditions, crypto.Hash(sci.ParentID), tb.wallet.keys[sci.UnlockConditions.UnlockHash()])
		if err != nil {
			return err
		}
	}

	// Add the exact output.
	newInput := types.SiacoinInput{
		ParentID:         parentTxn.SiacoinOutputID(0),
		UnlockConditions: parentUnlockConditions,
	}
	tb.parents = append(tb.parents, parentTxn)
	tb.siacoinInputs = append(tb.siacoinInputs, len(tb.transaction.SiacoinInputs))
	tb.transaction.SiacoinInputs = append(tb.transaction.SiacoinInputs, newInput)

	// Mark all outputs that were spent as spent.
	for _, scoid := range spentScoids {
		tb.wallet.spentOutputs[types.OutputID(scoid)] = tb.wallet.consensusSetHeight
	}
	return nil
}

// FundSiafunds will add a siafund input of exaclty 'amount' to the
// transaction. A parent transaction may be needed to achieve an input with the
// correct value. The siafund input will not be signed until 'Sign' is called
// on the transaction builder.
//
// TODO: The implementation of FundSiacoins is known to have quirks/bugs
// (non-fatal), and has diverged from the implementation of FundSiacoins. The
// implementations should be converged once again.
func (tb *transactionBuilder) FundSiafunds(amount types.Currency) error {
	lockID := tb.wallet.mu.Lock()
	defer tb.wallet.mu.Unlock(lockID)

	// Create and fund a parent transaction that will add the correct amount of
	// siafunds to the transaction.
	var fund types.Currency
	parentTxn := types.Transaction{}
	for scoid, sco := range tb.wallet.siafundOutputs {
		// Check that this output has not recently been spent by the wallet.
		spendHeight := tb.wallet.spentOutputs[types.OutputID(scoid)]
		if spendHeight > tb.wallet.consensusSetHeight-RespendTimeout {
			continue
		}
		outputUnlockConditions := tb.wallet.keys[sco.UnlockHash].UnlockConditions
		if tb.wallet.consensusSetHeight < outputUnlockConditions.Timelock {
			continue
		}
		// Mark the output as spent.
		tb.wallet.spentOutputs[types.OutputID(scoid)] = tb.wallet.consensusSetHeight

		// Add a siafund input for this output.
		parentClaimUnlockConditions, err := tb.wallet.nextPrimarySeedAddress()
		if err != nil {
			return err
		}
		sci := types.SiafundInput{
			ParentID:         scoid,
			UnlockConditions: outputUnlockConditions,
			ClaimUnlockHash:  parentClaimUnlockConditions.UnlockHash(),
		}
		parentTxn.SiafundInputs = append(parentTxn.SiafundInputs, sci)

		// Add the output to the total fund
		fund = fund.Add(sco.Value)
		if fund.Cmp(amount) >= 0 {
			break
		}
	}

	// Create and add the output that will be used to fund the standard
	// transaction.
	parentUnlockConditions, err := tb.wallet.nextPrimarySeedAddress()
	if err != nil {
		return err
	}
	exactOutput := types.SiafundOutput{
		Value:      amount,
		UnlockHash: parentUnlockConditions.UnlockHash(),
	}
	parentTxn.SiafundOutputs = append(parentTxn.SiafundOutputs, exactOutput)

	// Create a refund output if needed.
	if amount.Cmp(fund) != 0 {
		refundUnlockConditions, err := tb.wallet.nextPrimarySeedAddress()
		if err != nil {
			return err
		}
		refundOutput := types.SiafundOutput{
			Value:      fund.Sub(amount),
			UnlockHash: refundUnlockConditions.UnlockHash(),
		}
		parentTxn.SiafundOutputs = append(parentTxn.SiafundOutputs, refundOutput)
	}

	// Sign all of the inputs to the parent trancstion.
	for _, sfi := range parentTxn.SiafundInputs {
		err := addSignatures(&parentTxn, types.FullCoveredFields, sfi.UnlockConditions, crypto.Hash(sfi.ParentID), tb.wallet.keys[sfi.UnlockConditions.UnlockHash()])
		if err != nil {
			return err
		}
	}

	// Add the exact output.
	claimUnlockConditions, err := tb.wallet.nextPrimarySeedAddress()
	if err != nil {
		return err
	}
	newInput := types.SiafundInput{
		ParentID:         parentTxn.SiafundOutputID(0),
		UnlockConditions: parentUnlockConditions,
		ClaimUnlockHash:  claimUnlockConditions.UnlockHash(),
	}
	tb.parents = append(tb.parents, parentTxn)
	tb.siafundInputs = append(tb.siafundInputs, len(tb.transaction.SiafundInputs))
	tb.transaction.SiafundInputs = append(tb.transaction.SiafundInputs, newInput)
	return nil
}

// AddMinerFee adds a miner fee to the transaction, returning the index of the
// miner fee within the transaction.
func (tb *transactionBuilder) AddMinerFee(fee types.Currency) uint64 {
	tb.transaction.MinerFees = append(tb.transaction.MinerFees, fee)
	return uint64(len(tb.transaction.MinerFees) - 1)
}

// AddSiacoinInput adds a siacoin input to the transaction, returning the index
// of the siacoin input within the transaction. When 'Sign' gets called, this
// input will be left unsigned.
func (tb *transactionBuilder) AddSiacoinInput(input types.SiacoinInput) uint64 {
	tb.transaction.SiacoinInputs = append(tb.transaction.SiacoinInputs, input)
	return uint64(len(tb.transaction.SiacoinInputs) - 1)
}

// AddSiacoinOutput adds a siacoin output to the transaction, returning the
// index of the siacoin output within the transaction.
func (tb *transactionBuilder) AddSiacoinOutput(output types.SiacoinOutput) uint64 {
	tb.transaction.SiacoinOutputs = append(tb.transaction.SiacoinOutputs, output)
	return uint64(len(tb.transaction.SiacoinOutputs) - 1)
}

// AddFileContract adds a file contract to the transaction, returning the index
// of the file contract within the transaction.
func (tb *transactionBuilder) AddFileContract(fc types.FileContract) uint64 {
	tb.transaction.FileContracts = append(tb.transaction.FileContracts, fc)
	return uint64(len(tb.transaction.FileContracts) - 1)
}

// AddFileContractRevision adds a file contract revision to the transaction,
// returning the index of the file contract revision within the transaction.
// When 'Sign' gets called, this revision will be left unsigned.
func (tb *transactionBuilder) AddFileContractRevision(fcr types.FileContractRevision) uint64 {
	tb.transaction.FileContractRevisions = append(tb.transaction.FileContractRevisions, fcr)
	return uint64(len(tb.transaction.FileContractRevisions) - 1)
}

// AddStorageProof adds a storage proof to the transaction, returning the index
// of the storage proof within the transaction.
func (tb *transactionBuilder) AddStorageProof(sp types.StorageProof) uint64 {
	tb.transaction.StorageProofs = append(tb.transaction.StorageProofs, sp)
	return uint64(len(tb.transaction.StorageProofs) - 1)
}

// AddSiafundInput adds a siafund input to the transaction, returning the index
// of the siafund input within the transaction. When 'Sign' is called, this
// input will be left unsigned.
func (tb *transactionBuilder) AddSiafundInput(input types.SiafundInput) uint64 {
	tb.transaction.SiafundInputs = append(tb.transaction.SiafundInputs, input)
	return uint64(len(tb.transaction.SiafundInputs) - 1)
}

// AddSiafundOutput adds a siafund output to the transaction, returning the
// index of the siafund output within the transaction.
func (tb *transactionBuilder) AddSiafundOutput(output types.SiafundOutput) uint64 {
	tb.transaction.SiafundOutputs = append(tb.transaction.SiafundOutputs, output)
	return uint64(len(tb.transaction.SiafundOutputs) - 1)
}

// AddArbitraryData adds arbitrary data to the transaction, returning the index
// of the data within the transaction.
func (tb *transactionBuilder) AddArbitraryData(arb []byte) uint64 {
	tb.transaction.ArbitraryData = append(tb.transaction.ArbitraryData, arb)
	return uint64(len(tb.transaction.ArbitraryData) - 1)
}

// AddTransactionSignature adds a transaction signature to the transaction,
// returning the index of the signature within the transaction. The signature
// should already be valid, and shouldn't sign any of the inputs that were
// added by calling 'FundSiacoins' or 'FundSiafunds'.
func (tb *transactionBuilder) AddTransactionSignature(sig types.TransactionSignature) uint64 {
	tb.transaction.TransactionSignatures = append(tb.transaction.TransactionSignatures, sig)
	return uint64(len(tb.transaction.TransactionSignatures) - 1)
}

// Sign will sign any inputs added by 'FundSiacoins' or 'FundSiafunds' and
// return a transaction set that contains all parents prepended to the
// transaction. If more fields need to be added, a new transaction builder will
// need to be created.
//
// If the whole transaction flag  is set to true, then the whole transaction
// flag will be set in the covered fields object. If the whole transaction flag
// is set to false, then the covered fields object will cover all fields that
// have already been added to the transaction, but will also leave room for
// more fields to be added.
func (tb *transactionBuilder) Sign(wholeTransaction bool) ([]types.Transaction, error) {
	// Create the coveredfields struct.
	txn := tb.transaction
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

	// For each siacoin input in the transaction that we added, provide a
	// signature.
	lockID := tb.wallet.mu.Lock()
	defer tb.wallet.mu.Unlock(lockID)
	for _, inputIndex := range tb.siacoinInputs {
		input := txn.SiacoinInputs[inputIndex]
		key := tb.wallet.keys[input.UnlockConditions.UnlockHash()]
		err := addSignatures(&txn, coveredFields, input.UnlockConditions, crypto.Hash(input.ParentID), key)
		if err != nil {
			return nil, err
		}
	}
	for _, inputIndex := range tb.siafundInputs {
		input := txn.SiafundInputs[inputIndex]
		key := tb.wallet.keys[input.UnlockConditions.UnlockHash()]
		err := addSignatures(&txn, coveredFields, input.UnlockConditions, crypto.Hash(input.ParentID), key)
		if err != nil {
			return nil, err
		}
	}

	// Get the transaction set and delete the transaction from the registry.
	txnSet := append(tb.parents, txn)
	return txnSet, nil
}

// ViewTransaction returns a transaction-in-progress along with all of its
// parents, specified by id. An error is returned if the id is invalid.  Note
// that ids become invalid for a transaction after 'SignTransaction' has been
// called because the transaction gets deleted.
func (tb *transactionBuilder) View() (types.Transaction, []types.Transaction) {
	return tb.transaction, tb.parents
}
