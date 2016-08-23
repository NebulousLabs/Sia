package proto

import (
	"errors"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// estTxnSize is the estimated size of an encoded file contract
	// transaction set.
	estTxnSize = 2048
)

// FormContract forms a contract with a host and submits the contract
// transaction to tpool.
func FormContract(params ContractParams, txnBuilder transactionBuilder, tpool transactionPool) (modules.RenterContract, error) {
	// extract vars from params, for convenience
	host, filesize, startHeight, endHeight, refundAddress := params.Host, params.Filesize, params.StartHeight, params.EndHeight, params.RefundAddress

	// create our key
	ourSK, ourPK, err := crypto.GenerateKeyPair()
	if err != nil {
		return modules.RenterContract{}, err
	}
	// create unlock conditions
	uc := types.UnlockConditions{
		PublicKeys: []types.SiaPublicKey{
			types.Ed25519PublicKey(ourPK),
			host.PublicKey,
		},
		SignaturesRequired: 2,
	}

	// calculate cost to renter and cost to host
	// TODO: clarify/abstract this math
	storageAllocation := host.StoragePrice.Mul64(filesize).Mul64(uint64(endHeight - startHeight))
	hostCollateral := host.Collateral.Mul64(filesize).Mul64(uint64(endHeight - startHeight))
	if hostCollateral.Cmp(host.MaxCollateral) > 0 {
		// TODO: if we have to cap the collateral, it probably means we shouldn't be using this host
		// (ok within a factor of 2)
		hostCollateral = host.MaxCollateral
	}
	hostPayout := hostCollateral.Add(host.ContractPrice)
	payout := storageAllocation.Add(hostPayout).Mul64(10406).Div64(10000) // renter pays for siafund fee
	renterCost := payout.Sub(hostCollateral)

	// check for negative currency
	if types.PostTax(startHeight, payout).Cmp(hostPayout) < 0 {
		return modules.RenterContract{}, errors.New("payout smaller than host payout")
	}

	// create file contract
	fc := types.FileContract{
		FileSize:       0,
		FileMerkleRoot: crypto.Hash{}, // no proof possible without data
		WindowStart:    endHeight,
		WindowEnd:      endHeight + host.WindowSize,
		Payout:         payout,
		UnlockHash:     uc.UnlockHash(),
		RevisionNumber: 0,
		ValidProofOutputs: []types.SiacoinOutput{
			// outputs need to account for tax
			{Value: types.PostTax(startHeight, payout).Sub(hostPayout), UnlockHash: refundAddress},
			// collateral is returned to host
			{Value: hostPayout, UnlockHash: host.UnlockHash},
		},
		MissedProofOutputs: []types.SiacoinOutput{
			// same as above
			{Value: types.PostTax(startHeight, payout).Sub(hostPayout), UnlockHash: refundAddress},
			// same as above
			{Value: hostPayout, UnlockHash: host.UnlockHash},
			// once we start doing revisions, we'll move some coins to the host and some to the void
			{Value: types.ZeroCurrency, UnlockHash: types.UnlockHash{}},
		},
	}

	// calculate transaction fee
	_, maxFee := tpool.FeeEstimation()
	fee := maxFee.Mul64(estTxnSize)

	// build transaction containing fc
	err = txnBuilder.FundSiacoins(renterCost.Add(fee))
	if err != nil {
		return modules.RenterContract{}, err
	}
	txnBuilder.AddFileContract(fc)

	// add miner fee
	txnBuilder.AddMinerFee(fee)

	// create initial transaction set
	txn, parentTxns := txnBuilder.View()
	txnSet := append(parentTxns, txn)

	// initiate connection
	conn, err := net.DialTimeout("tcp", string(host.NetAddress), 15*time.Second)
	if err != nil {
		return modules.RenterContract{}, err
	}
	defer func() { _ = conn.Close() }()

	// allot time for sending RPC ID + verifySettings
	extendDeadline(conn, modules.NegotiateSettingsTime)
	if err = encoding.WriteObject(conn, modules.RPCFormContract); err != nil {
		return modules.RenterContract{}, err
	}

	// verify the host's settings and confirm its identity
	host, err = verifySettings(conn, host)
	if err != nil {
		return modules.RenterContract{}, err
	}
	if !host.AcceptingContracts {
		return modules.RenterContract{}, errors.New("host is not accepting contracts")
	}

	// allot time for negotiation
	extendDeadline(conn, modules.NegotiateFileContractTime)

	// send acceptance, txn signed by us, and pubkey
	if err = modules.WriteNegotiationAcceptance(conn); err != nil {
		return modules.RenterContract{}, errors.New("couldn't send initial acceptance: " + err.Error())
	}
	if err = encoding.WriteObject(conn, txnSet); err != nil {
		return modules.RenterContract{}, errors.New("couldn't send the contract signed by us: " + err.Error())
	}
	if err = encoding.WriteObject(conn, ourSK.PublicKey()); err != nil {
		return modules.RenterContract{}, errors.New("couldn't send our public key: " + err.Error())
	}

	// read acceptance and txn signed by host
	if err = modules.ReadNegotiationAcceptance(conn); err != nil {
		return modules.RenterContract{}, errors.New("host did not accept our proposed contract: " + err.Error())
	}
	// host now sends any new parent transactions, inputs and outputs that
	// were added to the transaction
	var newParents []types.Transaction
	var newInputs []types.SiacoinInput
	var newOutputs []types.SiacoinOutput
	if err = encoding.ReadObject(conn, &newParents, types.BlockSizeLimit); err != nil {
		return modules.RenterContract{}, errors.New("couldn't read the host's added parents: " + err.Error())
	}
	if err = encoding.ReadObject(conn, &newInputs, types.BlockSizeLimit); err != nil {
		return modules.RenterContract{}, errors.New("couldn't read the host's added inputs: " + err.Error())
	}
	if err = encoding.ReadObject(conn, &newOutputs, types.BlockSizeLimit); err != nil {
		return modules.RenterContract{}, errors.New("couldn't read the host's added outputs: " + err.Error())
	}

	// merge txnAdditions with txnSet
	txnBuilder.AddParents(newParents)
	for _, input := range newInputs {
		txnBuilder.AddSiacoinInput(input)
	}
	for _, output := range newOutputs {
		txnBuilder.AddSiacoinOutput(output)
	}

	// sign the txn
	signedTxnSet, err := txnBuilder.Sign(true)
	if err != nil {
		return modules.RenterContract{}, modules.WriteNegotiationRejection(conn, errors.New("failed to sign transaction: "+err.Error()))
	}

	// calculate signatures added by the transaction builder
	var addedSignatures []types.TransactionSignature
	_, _, _, addedSignatureIndices := txnBuilder.ViewAdded()
	for _, i := range addedSignatureIndices {
		addedSignatures = append(addedSignatures, signedTxnSet[len(signedTxnSet)-1].TransactionSignatures[i])
	}

	// create initial (no-op) revision, transaction, and signature
	initRevision := types.FileContractRevision{
		ParentID:          signedTxnSet[len(signedTxnSet)-1].FileContractID(0),
		UnlockConditions:  uc,
		NewRevisionNumber: 1,

		NewFileSize:           fc.FileSize,
		NewFileMerkleRoot:     fc.FileMerkleRoot,
		NewWindowStart:        fc.WindowStart,
		NewWindowEnd:          fc.WindowEnd,
		NewValidProofOutputs:  fc.ValidProofOutputs,
		NewMissedProofOutputs: fc.MissedProofOutputs,
		NewUnlockHash:         fc.UnlockHash,
	}
	renterRevisionSig := types.TransactionSignature{
		ParentID:       crypto.Hash(initRevision.ParentID),
		PublicKeyIndex: 0,
		CoveredFields: types.CoveredFields{
			FileContractRevisions: []uint64{0},
		},
	}
	revisionTxn := types.Transaction{
		FileContractRevisions: []types.FileContractRevision{initRevision},
		TransactionSignatures: []types.TransactionSignature{renterRevisionSig},
	}
	encodedSig, err := crypto.SignHash(revisionTxn.SigHash(0), ourSK)
	if err != nil {
		return modules.RenterContract{}, modules.WriteNegotiationRejection(conn, errors.New("failed to sign revision transaction: "+err.Error()))
	}
	revisionTxn.TransactionSignatures[0].Signature = encodedSig[:]

	// Send acceptance and signatures
	if err = modules.WriteNegotiationAcceptance(conn); err != nil {
		return modules.RenterContract{}, errors.New("couldn't send transaction acceptance: " + err.Error())
	}
	if err = encoding.WriteObject(conn, addedSignatures); err != nil {
		return modules.RenterContract{}, errors.New("couldn't send added signatures: " + err.Error())
	}
	if err = encoding.WriteObject(conn, revisionTxn.TransactionSignatures[0]); err != nil {
		return modules.RenterContract{}, errors.New("couldn't send revision signature: " + err.Error())
	}

	// Read the host acceptance and signatures.
	err = modules.ReadNegotiationAcceptance(conn)
	if err != nil {
		return modules.RenterContract{}, errors.New("host did not accept our signatures: " + err.Error())
	}
	var hostSigs []types.TransactionSignature
	if err = encoding.ReadObject(conn, &hostSigs, 2e3); err != nil {
		return modules.RenterContract{}, errors.New("couldn't read the host's signatures: " + err.Error())
	}
	for _, sig := range hostSigs {
		txnBuilder.AddTransactionSignature(sig)
	}
	var hostRevisionSig types.TransactionSignature
	if err = encoding.ReadObject(conn, &hostRevisionSig, 2e3); err != nil {
		return modules.RenterContract{}, errors.New("couldn't read the host's revision signature: " + err.Error())
	}
	revisionTxn.TransactionSignatures = append(revisionTxn.TransactionSignatures, hostRevisionSig)

	// Construct the final transaction.
	txn, parentTxns = txnBuilder.View()
	txnSet = append(parentTxns, txn)

	// Submit to blockchain.
	err = tpool.AcceptTransactionSet(txnSet)
	if err == modules.ErrDuplicateTransactionSet {
		// as long as it made it into the transaction pool, we're good
		err = nil
	}
	if err != nil {
		return modules.RenterContract{}, err
	}

	// calculate contract ID
	fcid := txn.FileContractID(0)

	return modules.RenterContract{
		FileContract:    fc,
		ID:              fcid,
		LastRevision:    initRevision,
		LastRevisionTxn: revisionTxn,
		NetAddress:      host.NetAddress,
		SecretKey:       ourSK,
	}, nil
}
