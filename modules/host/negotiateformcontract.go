package host

import (
	"net"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	// errCollateralBudgetExceeded is returned if the host does not have enough
	// room in the collateral budget to accept a particular file contract.
	errCollateralBudgetExceeded = ErrorInternal("host has reached its collateral budget and cannot accept the file contract")

	// errMaxCollateralReached is returned if a file contract is provided which
	// would require the host to supply more collateral than the host allows
	// per file contract.
	errMaxCollateralReached = ErrorInternal("file contract proposal expects the host to pay more than the maximum allowed collateral")
)

// contractCollateral returns the amount of collateral that the host is
// expected to add to the file contract based on the payout of the file
// contract and based on the host settings.
func contractCollateral(settings modules.HostExternalSettings, fc types.FileContract) types.Currency {
	return fc.ValidProofOutputs[1].Value.Sub(settings.ContractPrice)
}

// managedAddCollateral adds the host's collateral to the file contract
// transaction set, returning the new inputs and outputs that get added to the
// transaction, as well as any new parents that get added to the transaction
// set. The builder that is used to add the collateral is also returned,
// because the new transaction has not yet been signed.
func (h *Host) managedAddCollateral(settings modules.HostExternalSettings, txnSet []types.Transaction) (builder modules.TransactionBuilder, newParents []types.Transaction, newInputs []types.SiacoinInput, newOutputs []types.SiacoinOutput, err error) {
	txn := txnSet[len(txnSet)-1]
	parents := txnSet[:len(txnSet)-1]
	fc := txn.FileContracts[0]
	hostPortion := contractCollateral(settings, fc)
	builder, err = h.wallet.RegisterTransaction(txn, parents)
	if err != nil {
		return
	}
	err = builder.FundSiacoins(hostPortion)
	if err != nil {
		builder.Drop()
		return nil, nil, nil, nil, extendErr("could not add collateral: ", ErrorInternal(err.Error()))
	}

	// Return which inputs and outputs have been added by the collateral call.
	newParentIndices, newInputIndices, newOutputIndices, _ := builder.ViewAdded()
	updatedTxn, updatedParents := builder.View()
	for _, parentIndex := range newParentIndices {
		newParents = append(newParents, updatedParents[parentIndex])
	}
	for _, inputIndex := range newInputIndices {
		newInputs = append(newInputs, updatedTxn.SiacoinInputs[inputIndex])
	}
	for _, outputIndex := range newOutputIndices {
		newOutputs = append(newOutputs, updatedTxn.SiacoinOutputs[outputIndex])
	}
	return builder, newParents, newInputs, newOutputs, nil
}

// managedRPCFormContract accepts a file contract from a renter, checks the
// file contract for compliance with the host settings, and then commits to the
// file contract, creating a storage obligation and submitting the contract to
// the blockchain.
func (h *Host) managedRPCFormContract(conn net.Conn) error {
	// Send the host settings to the renter.
	err := h.managedRPCSettings(conn)
	if err != nil {
		return extendErr("failed RPCSettings: ", err)
	}
	// If the host is not accepting contracts, the connection can be closed.
	// The renter has been given enough information in the host settings to
	// understand that the connection is going to be closed.
	h.mu.Lock()
	settings := h.externalSettings()
	h.mu.Unlock()
	if !settings.AcceptingContracts {
		h.log.Debugln("Turning down contract because the host is not accepting contracts.")
		return nil
	}

	// Extend the deadline to meet the rest of file contract negotiation.
	conn.SetDeadline(time.Now().Add(modules.NegotiateFileContractTime))

	// The renter will either accept or reject the host's settings.
	err = modules.ReadNegotiationAcceptance(conn)
	if err != nil {
		return extendErr("renter did not accept settings: ", ErrorCommunication(err.Error()))
	}
	// If the renter sends an acceptance of the settings, it will be followed
	// by an unsigned transaction containing funding from the renter and a file
	// contract which matches what the final file contract should look like.
	// After the file contract, the renter will send a public key which is the
	// renter's public key in the unlock conditions that protect the file
	// contract from revision.
	var txnSet []types.Transaction
	var renterPK crypto.PublicKey
	err = encoding.ReadObject(conn, &txnSet, modules.NegotiateMaxFileContractSetLen)
	if err != nil {
		return extendErr("could not read renter transaction set: ", ErrorConnection(err.Error()))
	}
	err = encoding.ReadObject(conn, &renterPK, modules.NegotiateMaxSiaPubkeySize)
	if err != nil {
		return extendErr("could not read renter public key: ", ErrorConnection(err.Error()))
	}

	// The host verifies that the file contract coming over the wire is
	// acceptable.
	err = h.managedVerifyNewContract(txnSet, renterPK, settings)
	if err != nil {
		// The incoming file contract is not acceptable to the host, indicate
		// why to the renter.
		modules.WriteNegotiationRejection(conn, err) // Error ignored to preserve type in extendErr
		return extendErr("contract verification failed: ", err)
	}
	// The host adds collateral to the transaction.
	txnBuilder, newParents, newInputs, newOutputs, err := h.managedAddCollateral(settings, txnSet)
	if err != nil {
		modules.WriteNegotiationRejection(conn, err) // Error ignored to preserve type in extendErr
		return extendErr("failed to add collateral: ", err)
	}
	// The host indicates acceptance, and then sends any new parent
	// transactions, inputs and outputs that were added to the transaction.
	err = modules.WriteNegotiationAcceptance(conn)
	if err != nil {
		return extendErr("accepting verified contract failed: ", ErrorConnection(err.Error()))
	}
	err = encoding.WriteObject(conn, newParents)
	if err != nil {
		return extendErr("failed to write new parents: ", ErrorConnection(err.Error()))
	}
	err = encoding.WriteObject(conn, newInputs)
	if err != nil {
		return extendErr("failed to write new inputs: ", ErrorConnection(err.Error()))
	}
	err = encoding.WriteObject(conn, newOutputs)
	if err != nil {
		return extendErr("failed to write new outputs: ", ErrorConnection(err.Error()))
	}

	// The renter will now send a negotiation response, followed by transaction
	// signatures for the file contract transaction in the case of acceptance.
	// The transaction signatures will be followed by another transaction
	// signature, to sign a no-op file contract revision.
	err = modules.ReadNegotiationAcceptance(conn)
	if err != nil {
		return extendErr("renter did not accept updated transactions: ", ErrorCommunication(err.Error()))
	}
	var renterTxnSignatures []types.TransactionSignature
	var renterRevisionSignature types.TransactionSignature
	err = encoding.ReadObject(conn, &renterTxnSignatures, modules.NegotiateMaxTransactionSignaturesSize)
	if err != nil {
		return extendErr("could not read renter transaction signatures: ", ErrorConnection(err.Error()))
	}
	err = encoding.ReadObject(conn, &renterRevisionSignature, modules.NegotiateMaxTransactionSignatureSize)
	if err != nil {
		return extendErr("could not read renter revision signatures: ", ErrorConnection(err.Error()))
	}

	// The host adds the renter transaction signatures, then signs the
	// transaction and submits it to the blockchain, creating a storage
	// obligation in the process. The host's part is done before anything is
	// written to the renter, but to give the renter confidence, the host will
	// send the signatures so that the renter can immediately have the
	// completed file contract.
	//
	// During finalization, the signature for the revision is also checked, and
	// signatures for the revision transaction are created.
	h.mu.RLock()
	hostCollateral := contractCollateral(settings, txnSet[len(txnSet)-1].FileContracts[0])
	h.mu.RUnlock()
	hostTxnSignatures, hostRevisionSignature, newSOID, err := h.managedFinalizeContract(txnBuilder, renterPK, renterTxnSignatures, renterRevisionSignature, nil, hostCollateral, types.ZeroCurrency, types.ZeroCurrency, settings)
	if err != nil {
		// The incoming file contract is not acceptable to the host, indicate
		// why to the renter.
		modules.WriteNegotiationRejection(conn, err) // Error ignored to preserve type in extendErr
		return extendErr("contract finalization failed: ", err)
	}
	defer h.managedUnlockStorageObligation(newSOID)
	err = modules.WriteNegotiationAcceptance(conn)
	if err != nil {
		return extendErr("failed to write acceptance after contract finalization: ", ErrorConnection(err.Error()))
	}
	// The host sends the transaction signatures to the renter, followed by the
	// revision signature. Negotiation is complete.
	err = encoding.WriteObject(conn, hostTxnSignatures)
	if err != nil {
		return extendErr("failed to write host transaction signatures: ", ErrorConnection(err.Error()))
	}
	err = encoding.WriteObject(conn, hostRevisionSignature)
	if err != nil {
		return extendErr("failed to write host revision signatures: ", ErrorConnection(err.Error()))
	}
	return nil
}

// managedVerifyNewContract checks that an incoming file contract matches the host's
// expectations for a valid contract.
func (h *Host) managedVerifyNewContract(txnSet []types.Transaction, renterPK crypto.PublicKey, eSettings modules.HostExternalSettings) error {
	// Check that the transaction set is not empty.
	if len(txnSet) < 1 {
		return extendErr("zero-length transaction set: ", errEmptyObject)
	}
	// Check that there is a file contract in the txnSet.
	if len(txnSet[len(txnSet)-1].FileContracts) < 1 {
		return extendErr("transaction without file contract: ", errEmptyObject)
	}

	h.mu.RLock()
	blockHeight := h.blockHeight
	lockedStorageCollateral := h.financialMetrics.LockedStorageCollateral
	publicKey := h.publicKey
	iSettings := h.settings
	unlockHash := h.unlockHash
	h.mu.RUnlock()
	fc := txnSet[len(txnSet)-1].FileContracts[0]

	// A new file contract should have a file size of zero.
	if fc.FileSize != 0 {
		return errBadFileSize
	}
	if fc.FileMerkleRoot != (crypto.Hash{}) {
		return errBadFileMerkleRoot
	}
	// WindowStart must be at least revisionSubmissionBuffer blocks into the
	// future.
	if fc.WindowStart <= blockHeight+revisionSubmissionBuffer {
		h.log.Debugf("A renter tried to form a contract that had a window start which was too soon. The contract started at %v, the current height is %v, the revisionSubmissionBuffer is %v, and the comparison was %v <= %v\n", fc.WindowStart, blockHeight, revisionSubmissionBuffer, fc.WindowStart, blockHeight+revisionSubmissionBuffer)
		return errEarlyWindow
	}
	// WindowEnd must be at least settings.WindowSize blocks after
	// WindowStart.
	if fc.WindowEnd < fc.WindowStart+eSettings.WindowSize {
		return errSmallWindow
	}
	// WindowStart must not be more than settings.MaxDuration blocks into the
	// future.
	if fc.WindowStart > blockHeight+eSettings.MaxDuration {
		return errLongDuration
	}

	// ValidProofOutputs shoud have 2 outputs (renter + host) and missed
	// outputs should have 3 (renter + host + void)
	if len(fc.ValidProofOutputs) != 2 || len(fc.MissedProofOutputs) != 3 {
		return errBadContractOutputCounts
	}
	// The unlock hashes of the valid and missed proof outputs for the host
	// must match the host's unlock hash. The third missed output should point
	// to the void.
	if fc.ValidProofOutputs[1].UnlockHash != unlockHash || fc.MissedProofOutputs[1].UnlockHash != unlockHash || fc.MissedProofOutputs[2].UnlockHash != (types.UnlockHash{}) {
		return errBadPayoutUnlockHashes
	}
	// Check that the payouts for the valid proof outputs and the missed proof
	// outputs are the same - this is important because no data has been added
	// to the file contract yet.
	if !fc.ValidProofOutputs[1].Value.Equals(fc.MissedProofOutputs[1].Value) {
		return errMismatchedHostPayouts
	}
	// Check that there's enough payout for the host to cover at least the
	// contract price. This will prevent negative currency panics when working
	// with the collateral.
	if fc.ValidProofOutputs[1].Value.Cmp(eSettings.ContractPrice) < 0 {
		return errLowHostValidOutput
	}
	// Check that the collateral does not exceed the maximum amount of
	// collateral allowed.
	expectedCollateral := contractCollateral(eSettings, fc)
	if expectedCollateral.Cmp(eSettings.MaxCollateral) > 0 {
		return errMaxCollateralReached
	}
	// Check that the host has enough room in the collateral budget to add this
	// collateral.
	if lockedStorageCollateral.Add(expectedCollateral).Cmp(iSettings.CollateralBudget) > 0 {
		return errCollateralBudgetExceeded
	}

	// The unlock hash for the file contract must match the unlock hash that
	// the host knows how to spend.
	expectedUH := types.UnlockConditions{
		PublicKeys: []types.SiaPublicKey{
			types.Ed25519PublicKey(renterPK),
			publicKey,
		},
		SignaturesRequired: 2,
	}.UnlockHash()
	if fc.UnlockHash != expectedUH {
		return errBadUnlockHash
	}

	// Check that the transaction set has enough fees on it to get into the
	// blockchain.
	setFee := modules.CalculateFee(txnSet)
	minFee, _ := h.tpool.FeeEstimation()
	if setFee.Cmp(minFee) < 0 {
		return errLowTransactionFees
	}
	return nil
}
