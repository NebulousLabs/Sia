package proto

import (
	"errors"
	"net"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// Renew negotiates a new contract for data already stored with a host, and
// submits the new contract transaction to tpool.
func Renew(contract modules.RenterContract, params ContractParams, txnBuilder transactionBuilder, tpool transactionPool, cancelChan chan struct{}) (modules.RenterContract, error) {
	// Extract vars from params, for convenience
	host, hostCollateral, renterFunds, startHeight, endHeight, refundAddress := params.Host, params.HostCollateral, params.RenterFunds, params.StartHeight, params.EndHeight, params.RefundAddress
	ourSK := contract.SecretKey

	// Determine the amount of time that is being added to the contract.
	timeExtension := uint64(endHeight - contract.LastRevision.NewWindowEnd) // TODO: May need to add host.WindowSize too.
	// Determine the cost of adding that much time to the existing data in the
	// contract.
	basePayment := host.StoragePrice.Mul64(contract.LastRevision.NewFileSize).Mul64(timeExtension)
	if basePayment.Cmp(renterFunds) > 0 {
		// The caller should have checked that the renter funds were enough to
		// cover the base payment for the contract.
		build.Critical("Provided renter funds are insufficient to cover base storage costs:", basePayment, renterFunds)
		return modules.RenterContract{}, errors.New("provided renter funds are insufficent to cover contract cost")
	}
	// Determine the amount of collateral that needs to be added to the data in
	// the existing contract.
	baseCollateral := host.Collateral.Mul64(contract.LastRevision.NewFileSize).Mul64(timeExtension)
	if baseCollateral.Cmp(hostCollateral) > 0 {
		// If the base collateral exceeds the renter's chosen collateral, it
		// means that the renter is optimizing to pay less in siafund fees. Have
		// the host put up the full collateral immediately.
		baseCollateral = hostCollateral
	}

	// Create the file contract.
	payout := renterFunds.Add(hostCollateral).Add(host.ContractPrice).Mul64(10406).Div64(10000) // renter covers siafund fee
	fc := types.FileContract{
		FileSize:       contract.LastRevision.NewFileSize,
		FileMerkleRoot: contract.LastRevision.NewFileMerkleRoot,
		WindowStart:    endHeight,
		WindowEnd:      endHeight + host.WindowSize,
		Payout:         payout,
		UnlockHash:     contract.LastRevision.NewUnlockHash,
		RevisionNumber: 0,
		ValidProofOutputs: []types.SiacoinOutput{
			// renter
			{Value: renterFunds.Sub(basePayment), UnlockHash: refundAddress},
			// host
			{Value: basePayment.Add(hostCollateral).Add(host.ContractPrice), UnlockHash: host.UnlockHash},
		},
		MissedProofOutputs: []types.SiacoinOutput{
			// renter
			{Value: renterFunds.Sub(basePayment), UnlockHash: refundAddress},
			// host gets its unused collateral back, plus the contract price
			{Value: hostCollateral.Add(host.ContractPrice).Sub(baseCollateral), UnlockHash: host.UnlockHash},
			// void gets the spent storage fees, plus the collateral being risked
			{Value: basePayment.Add(baseCollateral), UnlockHash: types.UnlockHash{}},
		},
	}

	// calculate transaction fee
	_, maxFee := tpool.FeeEstimation()
	txnFee := maxFee.Mul64(estTxnSize)

	// build transaction containing fc
	renterCost := payout.Sub(hostCollateral).Add(txnFee)
	err := txnBuilder.FundSiacoins(renterCost)
	if err != nil {
		return modules.RenterContract{}, err
	}
	txnBuilder.AddFileContract(fc)

	// add miner fee
	txnBuilder.AddMinerFee(txnFee)

	// create initial transaction set
	txn, parentTxns := txnBuilder.View()
	txnSet := append(parentTxns, txn)

	// initiate connection
	dialer := &net.Dialer{
		Cancel:  cancelChan,
		Timeout: dialHostTimeout,
	}
	conn, err := dialer.Dial("tcp", string(host.NetAddress))
	if err != nil {
		return modules.RenterContract{}, err
	}
	defer conn.Close()

	// allot time for sending RPC ID, verifyRecentRevision, and verifySettings
	extendDeadline(conn, modules.NegotiateRecentRevisionTime+modules.NegotiateSettingsTime)
	if err = encoding.WriteObject(conn, modules.RPCRenewContract); err != nil {
		return modules.RenterContract{}, errors.New("couldn't initiate RPC: " + err.Error())
	}
	// verify that both parties are renewing the same contract
	if err = verifyRecentRevision(conn, contract); err != nil {
		// don't add context; want to preserve the original error type so that
		// callers can check using IsRevisionMismatch
		return modules.RenterContract{}, err
	}
	// verify the host's settings and confirm its identity
	host, err = verifySettings(conn, host)
	if err != nil {
		return modules.RenterContract{}, errors.New("settings exchange failed: " + err.Error())
	}
	if !host.AcceptingContracts {
		return modules.RenterContract{}, errors.New("host is not accepting contracts")
	}

	// allot time for negotiation
	extendDeadline(conn, modules.NegotiateRenewContractTime)

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
		UnlockConditions:  contract.LastRevision.UnlockConditions,
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
	encodedSig := crypto.SignHash(revisionTxn.SigHash(0), ourSK)
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
		HostPublicKey:   host.PublicKey,
		ID:              fcid,
		LastRevision:    initRevision,
		LastRevisionTxn: revisionTxn,
		MerkleRoots:     contract.MerkleRoots,
		NetAddress:      host.NetAddress,
		SecretKey:       ourSK,
		StartHeight:     startHeight,

		TotalCost:   renterCost,
		ContractFee: host.ContractPrice,
		TxnFee:      txnFee,
		SiafundFee:  types.Tax(startHeight, fc.Payout),
	}, nil
}
