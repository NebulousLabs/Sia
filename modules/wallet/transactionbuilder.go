package wallet

import (
	"bytes"
	"errors"
	"sort"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/coreos/bbolt"
)

var (
	// errBuilderAlreadySigned indicates that the transaction builder has
	// already added at least one successful signature to the transaction,
	// meaning that future calls to Sign will result in an invalid transaction.
	errBuilderAlreadySigned = errors.New("sign has already been called on this transaction builder, multiple calls can cause issues")

	// errDustOutput indicates an output is not spendable because it is dust.
	errDustOutput = errors.New("output is too small")

	// errOutputTimelock indicates an output's timelock is still active.
	errOutputTimelock = errors.New("wallet consensus set height is lower than the output timelock")

	// errSpendHeightTooHigh indicates an output's spend height is greater than
	// the allowed height.
	errSpendHeightTooHigh = errors.New("output spend height exceeds the allowed height")
)

// transactionBuilder allows transactions to be manually constructed, including
// the ability to fund transactions with siacoins and siafunds from the wallet.
type transactionBuilder struct {
	// 'signed' indicates that at least one transaction signature has been
	// added to the wallet, meaning that future calls to 'Sign' will fail.
	parents     []types.Transaction
	signed      bool
	transaction types.Transaction

	newParents            []int
	siacoinInputs         []int
	siafundInputs         []int
	transactionSignatures []int

	wallet *Wallet
}

// addSignatures will sign a transaction using a spendable key, with support
// for multisig spendable keys. Because of the restricted input, the function
// is compatible with both siacoin inputs and siafund inputs.
func addSignatures(txn *types.Transaction, cf types.CoveredFields, uc types.UnlockConditions, parentID crypto.Hash, spendKey spendableKey) (newSigIndices []int) {
	// Try to find the matching secret key for each public key - some public
	// keys may not have a match. Some secret keys may be used multiple times,
	// which is why public keys are used as the outer loop.
	totalSignatures := uint64(0)
	for i, siaPubKey := range uc.PublicKeys {
		// Search for the matching secret key to the public key.
		for j := range spendKey.SecretKeys {
			pubKey := spendKey.SecretKeys[j].PublicKey()
			if !bytes.Equal(siaPubKey.Key, pubKey[:]) {
				continue
			}

			// Found the right secret key, add a signature.
			sig := types.TransactionSignature{
				ParentID:       parentID,
				CoveredFields:  cf,
				PublicKeyIndex: uint64(i),
			}
			newSigIndices = append(newSigIndices, len(txn.TransactionSignatures))
			txn.TransactionSignatures = append(txn.TransactionSignatures, sig)
			sigIndex := len(txn.TransactionSignatures) - 1
			sigHash := txn.SigHash(sigIndex)
			encodedSig := crypto.SignHash(sigHash, spendKey.SecretKeys[j])
			txn.TransactionSignatures[sigIndex].Signature = encodedSig[:]

			// Count that the signature has been added, and break out of the
			// secret key loop.
			totalSignatures++
			break
		}

		// If there are enough signatures to satisfy the unlock conditions,
		// break out of the outer loop.
		if totalSignatures == uc.SignaturesRequired {
			break
		}
	}
	return newSigIndices
}

// checkOutput is a helper function used to determine if an output is usable.
func (w *Wallet) checkOutput(tx *bolt.Tx, currentHeight types.BlockHeight, id types.SiacoinOutputID, output types.SiacoinOutput, dustThreshold types.Currency) error {
	// Check that an output is not dust
	if output.Value.Cmp(dustThreshold) < 0 {
		return errDustOutput
	}
	// Check that this output has not recently been spent by the wallet.
	spendHeight, err := dbGetSpentOutput(tx, types.OutputID(id))
	if err == nil {
		if spendHeight+RespendTimeout > currentHeight {
			return errSpendHeightTooHigh
		}
	}
	outputUnlockConditions := w.keys[output.UnlockHash].UnlockConditions
	if currentHeight < outputUnlockConditions.Timelock {
		return errOutputTimelock
	}

	return nil
}

// FundSiacoins will add a siacoin input of exactly 'amount' to the
// transaction. A parent transaction may be needed to achieve an input with the
// correct value. The siacoin input will not be signed until 'Sign' is called
// on the transaction builder.
func (tb *transactionBuilder) FundSiacoins(amount types.Currency) error {
	// dustThreshold has to be obtained separate from the lock
	dustThreshold := tb.wallet.DustThreshold()

	tb.wallet.mu.Lock()
	defer tb.wallet.mu.Unlock()

	consensusHeight, err := dbGetConsensusHeight(tb.wallet.dbTx)
	if err != nil {
		return err
	}

	// Collect a value-sorted set of siacoin outputs.
	var so sortedOutputs
	err = dbForEachSiacoinOutput(tb.wallet.dbTx, func(scoid types.SiacoinOutputID, sco types.SiacoinOutput) {
		so.ids = append(so.ids, scoid)
		so.outputs = append(so.outputs, sco)
	})
	if err != nil {
		return err
	}
	// Add all of the unconfirmed outputs as well.
	for _, upt := range tb.wallet.unconfirmedProcessedTransactions {
		for i, sco := range upt.Transaction.SiacoinOutputs {
			// Determine if the output belongs to the wallet.
			_, exists := tb.wallet.keys[sco.UnlockHash]
			if !exists {
				continue
			}
			so.ids = append(so.ids, upt.Transaction.SiacoinOutputID(uint64(i)))
			so.outputs = append(so.outputs, sco)
		}
	}
	sort.Sort(sort.Reverse(so))

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
		// Check that the output can be spent.
		if err := tb.wallet.checkOutput(tb.wallet.dbTx, consensusHeight, scoid, sco, dustThreshold); err != nil {
			if err == errSpendHeightTooHigh {
				potentialFund = potentialFund.Add(sco.Value)
			}
			continue
		}

		// Add a siacoin input for this output.
		sci := types.SiacoinInput{
			ParentID:         scoid,
			UnlockConditions: tb.wallet.keys[sco.UnlockHash].UnlockConditions,
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
	if potentialFund.Cmp(amount) >= 0 && fund.Cmp(amount) < 0 {
		return modules.ErrIncompleteTransactions
	}
	if fund.Cmp(amount) < 0 {
		return modules.ErrLowBalance
	}

	// Create and add the output that will be used to fund the standard
	// transaction.
	parentUnlockConditions, err := tb.wallet.nextPrimarySeedAddress(tb.wallet.dbTx)
	if err != nil {
		return err
	}

	exactOutput := types.SiacoinOutput{
		Value:      amount,
		UnlockHash: parentUnlockConditions.UnlockHash(),
	}
	parentTxn.SiacoinOutputs = append(parentTxn.SiacoinOutputs, exactOutput)

	// Create a refund output if needed.
	if !amount.Equals(fund) {
		refundUnlockConditions, err := tb.wallet.nextPrimarySeedAddress(tb.wallet.dbTx)
		if err != nil {
			return err
		}
		refundOutput := types.SiacoinOutput{
			Value:      fund.Sub(amount),
			UnlockHash: refundUnlockConditions.UnlockHash(),
		}
		parentTxn.SiacoinOutputs = append(parentTxn.SiacoinOutputs, refundOutput)
	}

	// Sign all of the inputs to the parent transaction.
	for _, sci := range parentTxn.SiacoinInputs {
		addSignatures(&parentTxn, types.FullCoveredFields, sci.UnlockConditions, crypto.Hash(sci.ParentID), tb.wallet.keys[sci.UnlockConditions.UnlockHash()])
	}
	// Mark the parent output as spent. Must be done after the transaction is
	// finished because otherwise the txid and output id will change.
	err = dbPutSpentOutput(tb.wallet.dbTx, types.OutputID(parentTxn.SiacoinOutputID(0)), consensusHeight)
	if err != nil {
		return err
	}

	// Add the exact output.
	newInput := types.SiacoinInput{
		ParentID:         parentTxn.SiacoinOutputID(0),
		UnlockConditions: parentUnlockConditions,
	}
	tb.newParents = append(tb.newParents, len(tb.parents))
	tb.parents = append(tb.parents, parentTxn)
	tb.siacoinInputs = append(tb.siacoinInputs, len(tb.transaction.SiacoinInputs))
	tb.transaction.SiacoinInputs = append(tb.transaction.SiacoinInputs, newInput)

	// Mark all outputs that were spent as spent.
	for _, scoid := range spentScoids {
		err = dbPutSpentOutput(tb.wallet.dbTx, types.OutputID(scoid), consensusHeight)
		if err != nil {
			return err
		}
	}
	return nil
}

// FundSiafunds will add a siafund input of exactly 'amount' to the
// transaction. A parent transaction may be needed to achieve an input with the
// correct value. The siafund input will not be signed until 'Sign' is called
// on the transaction builder.
func (tb *transactionBuilder) FundSiafunds(amount types.Currency) error {
	tb.wallet.mu.Lock()
	defer tb.wallet.mu.Unlock()

	consensusHeight, err := dbGetConsensusHeight(tb.wallet.dbTx)
	if err != nil {
		return err
	}

	// Create and fund a parent transaction that will add the correct amount of
	// siafunds to the transaction.
	var fund types.Currency
	var potentialFund types.Currency
	parentTxn := types.Transaction{}
	var spentSfoids []types.SiafundOutputID
	c := tb.wallet.dbTx.Bucket(bucketSiafundOutputs).Cursor()
	for idBytes, sfoBytes := c.First(); idBytes != nil; idBytes, sfoBytes = c.Next() {
		var sfoid types.SiafundOutputID
		var sfo types.SiafundOutput
		if err := encoding.Unmarshal(idBytes, &sfoid); err != nil {
			return err
		} else if err := encoding.Unmarshal(sfoBytes, &sfo); err != nil {
			return err
		}

		// Check that this output has not recently been spent by the wallet.
		spendHeight, err := dbGetSpentOutput(tb.wallet.dbTx, types.OutputID(sfoid))
		if err != nil {
			// mimic map behavior: no entry means zero value
			spendHeight = 0
		}
		// Prevent an underflow error.
		allowedHeight := consensusHeight - RespendTimeout
		if consensusHeight < RespendTimeout {
			allowedHeight = 0
		}
		if spendHeight > allowedHeight {
			potentialFund = potentialFund.Add(sfo.Value)
			continue
		}
		outputUnlockConditions := tb.wallet.keys[sfo.UnlockHash].UnlockConditions
		if consensusHeight < outputUnlockConditions.Timelock {
			continue
		}

		// Add a siafund input for this output.
		parentClaimUnlockConditions, err := tb.wallet.nextPrimarySeedAddress(tb.wallet.dbTx)
		if err != nil {
			return err
		}
		sfi := types.SiafundInput{
			ParentID:         sfoid,
			UnlockConditions: outputUnlockConditions,
			ClaimUnlockHash:  parentClaimUnlockConditions.UnlockHash(),
		}
		parentTxn.SiafundInputs = append(parentTxn.SiafundInputs, sfi)
		spentSfoids = append(spentSfoids, sfoid)

		// Add the output to the total fund
		fund = fund.Add(sfo.Value)
		potentialFund = potentialFund.Add(sfo.Value)
		if fund.Cmp(amount) >= 0 {
			break
		}
	}
	if potentialFund.Cmp(amount) >= 0 && fund.Cmp(amount) < 0 {
		return modules.ErrIncompleteTransactions
	}
	if fund.Cmp(amount) < 0 {
		return modules.ErrLowBalance
	}

	// Create and add the output that will be used to fund the standard
	// transaction.
	parentUnlockConditions, err := tb.wallet.nextPrimarySeedAddress(tb.wallet.dbTx)
	if err != nil {
		return err
	}
	exactOutput := types.SiafundOutput{
		Value:      amount,
		UnlockHash: parentUnlockConditions.UnlockHash(),
	}
	parentTxn.SiafundOutputs = append(parentTxn.SiafundOutputs, exactOutput)

	// Create a refund output if needed.
	if !amount.Equals(fund) {
		refundUnlockConditions, err := tb.wallet.nextPrimarySeedAddress(tb.wallet.dbTx)
		if err != nil {
			return err
		}
		refundOutput := types.SiafundOutput{
			Value:      fund.Sub(amount),
			UnlockHash: refundUnlockConditions.UnlockHash(),
		}
		parentTxn.SiafundOutputs = append(parentTxn.SiafundOutputs, refundOutput)
	}

	// Sign all of the inputs to the parent transaction.
	for _, sfi := range parentTxn.SiafundInputs {
		addSignatures(&parentTxn, types.FullCoveredFields, sfi.UnlockConditions, crypto.Hash(sfi.ParentID), tb.wallet.keys[sfi.UnlockConditions.UnlockHash()])
	}

	// Add the exact output.
	claimUnlockConditions, err := tb.wallet.nextPrimarySeedAddress(tb.wallet.dbTx)
	if err != nil {
		return err
	}
	newInput := types.SiafundInput{
		ParentID:         parentTxn.SiafundOutputID(0),
		UnlockConditions: parentUnlockConditions,
		ClaimUnlockHash:  claimUnlockConditions.UnlockHash(),
	}
	tb.newParents = append(tb.newParents, len(tb.parents))
	tb.parents = append(tb.parents, parentTxn)
	tb.siafundInputs = append(tb.siafundInputs, len(tb.transaction.SiafundInputs))
	tb.transaction.SiafundInputs = append(tb.transaction.SiafundInputs, newInput)

	// Mark all outputs that were spent as spent.
	for _, sfoid := range spentSfoids {
		err = dbPutSpentOutput(tb.wallet.dbTx, types.OutputID(sfoid), consensusHeight)
		if err != nil {
			return err
		}
	}
	return nil
}

// AddParents adds a set of parents to the transaction.
func (tb *transactionBuilder) AddParents(newParents []types.Transaction) {
	tb.parents = append(tb.parents, newParents...)
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

// Drop discards all of the outputs in a transaction, returning them to the
// pool so that other transactions may use them. 'Drop' should only be called
// if a transaction is both unsigned and will not be used any further.
func (tb *transactionBuilder) Drop() {
	tb.wallet.mu.Lock()
	defer tb.wallet.mu.Unlock()

	// Iterate through all parents and the transaction itself and restore all
	// outputs to the list of available outputs.
	txns := append(tb.parents, tb.transaction)
	for _, txn := range txns {
		for _, sci := range txn.SiacoinInputs {
			dbDeleteSpentOutput(tb.wallet.dbTx, types.OutputID(sci.ParentID))
		}
	}

	tb.parents = nil
	tb.signed = false
	tb.transaction = types.Transaction{}

	tb.newParents = nil
	tb.siacoinInputs = nil
	tb.siafundInputs = nil
	tb.transactionSignatures = nil
}

// Sign will sign any inputs added by 'FundSiacoins' or 'FundSiafunds' and
// return a transaction set that contains all parents prepended to the
// transaction. If more fields need to be added, a new transaction builder will
// need to be created.
//
// If the whole transaction flag is set to true, then the whole transaction
// flag will be set in the covered fields object. If the whole transaction flag
// is set to false, then the covered fields object will cover all fields that
// have already been added to the transaction, but will also leave room for
// more fields to be added.
//
// Sign should not be called more than once. If, for some reason, there is an
// error while calling Sign, the builder should be dropped.
func (tb *transactionBuilder) Sign(wholeTransaction bool) ([]types.Transaction, error) {
	if tb.signed {
		return nil, errBuilderAlreadySigned
	}

	// Create the coveredfields struct.
	var coveredFields types.CoveredFields
	if wholeTransaction {
		coveredFields = types.CoveredFields{WholeTransaction: true}
	} else {
		for i := range tb.transaction.MinerFees {
			coveredFields.MinerFees = append(coveredFields.MinerFees, uint64(i))
		}
		for i := range tb.transaction.SiacoinInputs {
			coveredFields.SiacoinInputs = append(coveredFields.SiacoinInputs, uint64(i))
		}
		for i := range tb.transaction.SiacoinOutputs {
			coveredFields.SiacoinOutputs = append(coveredFields.SiacoinOutputs, uint64(i))
		}
		for i := range tb.transaction.FileContracts {
			coveredFields.FileContracts = append(coveredFields.FileContracts, uint64(i))
		}
		for i := range tb.transaction.FileContractRevisions {
			coveredFields.FileContractRevisions = append(coveredFields.FileContractRevisions, uint64(i))
		}
		for i := range tb.transaction.StorageProofs {
			coveredFields.StorageProofs = append(coveredFields.StorageProofs, uint64(i))
		}
		for i := range tb.transaction.SiafundInputs {
			coveredFields.SiafundInputs = append(coveredFields.SiafundInputs, uint64(i))
		}
		for i := range tb.transaction.SiafundOutputs {
			coveredFields.SiafundOutputs = append(coveredFields.SiafundOutputs, uint64(i))
		}
		for i := range tb.transaction.ArbitraryData {
			coveredFields.ArbitraryData = append(coveredFields.ArbitraryData, uint64(i))
		}
	}
	// TransactionSignatures don't get covered by the 'WholeTransaction' flag,
	// and must be covered manually.
	for i := range tb.transaction.TransactionSignatures {
		coveredFields.TransactionSignatures = append(coveredFields.TransactionSignatures, uint64(i))
	}

	// For each siacoin input in the transaction that we added, provide a
	// signature.
	tb.wallet.mu.RLock()
	defer tb.wallet.mu.RUnlock()
	for _, inputIndex := range tb.siacoinInputs {
		input := tb.transaction.SiacoinInputs[inputIndex]
		key, ok := tb.wallet.keys[input.UnlockConditions.UnlockHash()]
		if !ok {
			return nil, errors.New("transaction builder added an input that it cannot sign")
		}
		newSigIndices := addSignatures(&tb.transaction, coveredFields, input.UnlockConditions, crypto.Hash(input.ParentID), key)
		tb.transactionSignatures = append(tb.transactionSignatures, newSigIndices...)
		tb.signed = true // Signed is set to true after one successful signature to indicate that future signings can cause issues.
	}
	for _, inputIndex := range tb.siafundInputs {
		input := tb.transaction.SiafundInputs[inputIndex]
		key, ok := tb.wallet.keys[input.UnlockConditions.UnlockHash()]
		if !ok {
			return nil, errors.New("transaction builder added an input that it cannot sign")
		}
		newSigIndices := addSignatures(&tb.transaction, coveredFields, input.UnlockConditions, crypto.Hash(input.ParentID), key)
		tb.transactionSignatures = append(tb.transactionSignatures, newSigIndices...)
		tb.signed = true // Signed is set to true after one successful signature to indicate that future signings can cause issues.
	}

	// Get the transaction set and delete the transaction from the registry.
	txnSet := append(tb.parents, tb.transaction)
	return txnSet, nil
}

// ViewTransaction returns a transaction-in-progress along with all of its
// parents, specified by id. An error is returned if the id is invalid.  Note
// that ids become invalid for a transaction after 'SignTransaction' has been
// called because the transaction gets deleted.
func (tb *transactionBuilder) View() (types.Transaction, []types.Transaction) {
	return tb.transaction, tb.parents
}

// ViewAdded returns all of the siacoin inputs, siafund inputs, and parent
// transactions that have been automatically added by the builder.
func (tb *transactionBuilder) ViewAdded() (newParents, siacoinInputs, siafundInputs, transactionSignatures []int) {
	return tb.newParents, tb.siacoinInputs, tb.siafundInputs, tb.transactionSignatures
}

// registerTransaction takes a transaction and its parents and returns a
// wallet.TransactionBuilder which can be used to expand the transaction. The
// most typical call is 'RegisterTransaction(types.Transaction{}, nil)', which
// registers a new transaction without parents.
func (w *Wallet) registerTransaction(t types.Transaction, parents []types.Transaction) *transactionBuilder {
	// Create a deep copy of the transaction and parents by encoding them. A
	// deep copy ensures that there are no pointer or slice related errors -
	// the builder will be working directly on the transaction, and the
	// transaction may be in use elsewhere (in this case, the host is using the
	// transaction.
	pBytes := encoding.Marshal(parents)
	var pCopy []types.Transaction
	err := encoding.Unmarshal(pBytes, &pCopy)
	if err != nil {
		panic(err)
	}
	tBytes := encoding.Marshal(t)
	var tCopy types.Transaction
	err = encoding.Unmarshal(tBytes, &tCopy)
	if err != nil {
		panic(err)
	}
	return &transactionBuilder{
		parents:     pCopy,
		transaction: tCopy,

		wallet: w,
	}
}

// RegisterTransaction takes a transaction and its parents and returns a
// modules.TransactionBuilder which can be used to expand the transaction. The
// most typical call is 'RegisterTransaction(types.Transaction{}, nil)', which
// registers a new transaction without parents.
func (w *Wallet) RegisterTransaction(t types.Transaction, parents []types.Transaction) modules.TransactionBuilder {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.registerTransaction(t, parents)
}

// StartTransaction is a convenience function that calls
// RegisterTransaction(types.Transaction{}, nil).
func (w *Wallet) StartTransaction() modules.TransactionBuilder {
	return w.RegisterTransaction(types.Transaction{}, nil)
}

// SpendableOutputs returns the outputs spendable by the wallet. For each
// output, MaturityHeight is the height of the block containing the output.
func (w *Wallet) SpendableOutputs() []modules.ProcessedOutput {
	w.mu.Lock()
	defer w.mu.Unlock()
	// ensure durability of reported outputs
	w.syncDB()

	var outputs []modules.ProcessedOutput
	dbForEachSiacoinOutput(w.dbTx, func(scoid types.SiacoinOutputID, sco types.SiacoinOutput) {
		outputs = append(outputs, modules.ProcessedOutput{
			FundType:       types.SpecifierSiacoinOutput,
			ID:             types.OutputID(scoid),
			RelatedAddress: sco.UnlockHash,
			Value:          sco.Value,
			WalletAddress:  true,
		})
	})
	dbForEachSiafundOutput(w.dbTx, func(sfoid types.SiafundOutputID, sfo types.SiafundOutput) {
		outputs = append(outputs, modules.ProcessedOutput{
			FundType:       types.SpecifierSiafundOutput,
			ID:             types.OutputID(sfoid),
			RelatedAddress: sfo.UnlockHash,
			Value:          sfo.Value,
			WalletAddress:  true,
		})
	})

	// lookup the confirmation height of each output
	// TODO: would be much better to store this alongside outputs
	for i, o := range outputs {
		txnIndices, _ := dbGetAddrTransactions(w.dbTx, o.RelatedAddress)
		for _, j := range txnIndices {
			pt, err := dbGetProcessedTransaction(w.dbTx, j)
			if err != nil {
				continue
			}
			for _, sco := range pt.Outputs {
				if sco.ID == o.ID {
					outputs[i].MaturityHeight = pt.ConfirmationHeight
					break
				}
			}
		}
	}

	return outputs
}

// SignTransaction signs txn using secret keys controlled by w, which must be
// unlocked. For each SiacoinInput whose UnlockConditions are not set,
// SignTransaction attempts to fill in the UnlockConditions and adds a
// corresponding signature. It returns the indices of each signed input.
func (w *Wallet) SignTransaction(txn *types.Transaction) []int {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.unlocked {
		return nil
	}

	var signed []int
	for i, sci := range txn.SiacoinInputs {
		// identify inputs with unset UnlockConditions
		if sci.UnlockConditions.SignaturesRequired == 0 {
			// locate output corresponding to this input
			sco, err := dbGetSiacoinOutput(w.dbTx, sci.ParentID)
			if err != nil {
				continue
			}
			// lookup the signing key(s)
			sk, ok := w.keys[sco.UnlockHash]
			if !ok {
				w.log.Critical("wallet is missing a signing key")
				continue
			}
			txn.SiacoinInputs[i].UnlockConditions = sk.UnlockConditions
			cf := types.CoveredFields{WholeTransaction: true}
			addSignatures(txn, cf, sk.UnlockConditions, crypto.Hash(sci.ParentID), sk)
			signed = append(signed, i)
		}
	}
	return signed
}
