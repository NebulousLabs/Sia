package host

import (
	"errors"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	// errRenewDoesNotExtend is returned if a file contract renewal is
	// presented which does not extend the existing file contract.
	errRenewDoesNotExtend = errors.New("file contract renewal does not extend the existing file contract")
)

// renewBaseCollateral returns the base collateral on the storage in the file
// contract, using the host's external settings and the starting file contract.
func renewBaseCollateral(so storageObligation, settings modules.HostExternalSettings, fc types.FileContract) types.Currency {
	if fc.WindowEnd <= so.proofDeadline() {
		return types.NewCurrency64(0)
	}
	timeExtension := fc.WindowEnd - so.proofDeadline()
	return settings.Collateral.Mul64(fc.FileSize).Mul64(uint64(timeExtension))
}

// renewBasePrice returns the base cost of the storage in the file contract,
// using the host external settings and the starting file contract.
func renewBasePrice(so storageObligation, settings modules.HostExternalSettings, fc types.FileContract) types.Currency {
	if fc.WindowEnd <= so.proofDeadline() {
		return types.NewCurrency64(0)
	}
	timeExtension := fc.WindowEnd - so.proofDeadline()
	return settings.StoragePrice.Mul64(fc.FileSize).Mul64(uint64(timeExtension))
}

// renewContractCollateral returns the amount of collateral that the host is
// expected to add to the file contract based on the file contract and host
// settings.
func renewContractCollateral(so storageObligation, settings modules.HostExternalSettings, fc types.FileContract) types.Currency {
	return fc.ValidProofOutputs[1].Value.Sub(settings.ContractPrice).Sub(renewBasePrice(so, settings, fc))
}

// managedAddRenewCollateral adds the host's collateral to the renewed file
// contract.
func (h *Host) managedAddRenewCollateral(so storageObligation, settings modules.HostExternalSettings, txnSet []types.Transaction) (builder modules.TransactionBuilder, newParents []types.Transaction, newInputs []types.SiacoinInput, newOutputs []types.SiacoinOutput, err error) {
	txn := txnSet[len(txnSet)-1]
	parents := txnSet[:len(txnSet)-1]
	fc := txn.FileContracts[0]
	hostPortion := renewContractCollateral(so, settings, fc)
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

// managedRenewContract accepts a request to renew a file contract.
func (h *Host) managedRPCRenewContract(conn net.Conn) error {
	// Perform the recent revision protocol to get the file contract being
	// revised.
	_, so, err := h.managedRPCRecentRevision(conn)
	if err != nil {
		return extendErr("failed RPCRecentRevision during RPCRenewContract: ", err)
	}
	// The storage obligation is received with a lock. Defer a call to unlock
	// the storage obligation.
	defer func() {
		h.managedUnlockStorageObligation(so.id())
	}()

	// Perform the host settings exchange with the renter.
	err = h.managedRPCSettings(conn)
	if err != nil {
		return extendErr("RPCSettings failed: ", err)
	}

	// Set the renewal deadline.
	conn.SetDeadline(time.Now().Add(modules.NegotiateRenewContractTime))

	// The renter will either accept or reject the host's settings.
	err = modules.ReadNegotiationAcceptance(conn)
	if err != nil {
		return extendErr("renter rejected the host settings: ", ErrorCommunication(err.Error()))
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
		return extendErr("unable to read transaction set: ", ErrorConnection(err.Error()))
	}
	err = encoding.ReadObject(conn, &renterPK, modules.NegotiateMaxSiaPubkeySize)
	if err != nil {
		return extendErr("unable to read renter public key: ", ErrorConnection(err.Error()))
	}

	h.mu.Lock()
	settings := h.externalSettings()
	h.mu.Unlock()

	// Verify that the transaction coming over the wire is a proper renewal.
	err = h.managedVerifyRenewedContract(so, txnSet, renterPK)
	if err != nil {
		modules.WriteNegotiationRejection(conn, err) // Error is ignored to preserve type for extendErr
		return extendErr("verification of renewal failed: ", err)
	}
	txnBuilder, newParents, newInputs, newOutputs, err := h.managedAddRenewCollateral(so, settings, txnSet)
	if err != nil {
		modules.WriteNegotiationRejection(conn, err) // Error is ignored to preserve type for extendErr
		return extendErr("failed to add collateral: ", err)
	}
	// The host indicates acceptance, then sends the new parents, inputs, and
	// outputs to the transaction.
	err = modules.WriteNegotiationAcceptance(conn)
	if err != nil {
		return extendErr("failed to write acceptance: ", ErrorConnection(err.Error()))
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

	// The renter will send a negotiation response, followed by transaction
	// signatures for the file contract transaction in the case of acceptance.
	// The transaction signatures will be followed by another transaction
	// signature to sign the no-op file contract revision associated with the
	// new file contract.
	err = modules.ReadNegotiationAcceptance(conn)
	if err != nil {
		return extendErr("renter rejected collateral extension: ", ErrorCommunication(err.Error()))
	}
	var renterTxnSignatures []types.TransactionSignature
	var renterRevisionSignature types.TransactionSignature
	err = encoding.ReadObject(conn, &renterTxnSignatures, modules.NegotiateMaxTransactionSignatureSize)
	if err != nil {
		return extendErr("failed to read renter transaction signatures: ", ErrorConnection(err.Error()))
	}
	err = encoding.ReadObject(conn, &renterRevisionSignature, modules.NegotiateMaxTransactionSignatureSize)
	if err != nil {
		return extendErr("failed to read renter revision signatures: ", ErrorConnection(err.Error()))
	}

	// The host adds the renter transaction signatures, then signs the
	// transaction and submits it to the blockchain, creating a storage
	// obligation in the process. The host's part is now complete and the
	// contract is finalized, but to give confidence to the renter the host
	// will send the signatures so that the renter can immediately have the
	// completed file contract.
	//
	// During finalization the signatures sent by the renter are all checked.
	h.mu.RLock()
	fc := txnSet[len(txnSet)-1].FileContracts[0]
	renewCollateral := renewContractCollateral(so, settings, fc)
	renewRevenue := renewBasePrice(so, settings, fc)
	renewRisk := renewBaseCollateral(so, settings, fc)
	h.mu.RUnlock()
	hostTxnSignatures, hostRevisionSignature, newSOID, err := h.managedFinalizeContract(txnBuilder, renterPK, renterTxnSignatures, renterRevisionSignature, so.SectorRoots, renewCollateral, renewRevenue, renewRisk, settings)
	if err != nil {
		modules.WriteNegotiationRejection(conn, err) // Error is ignored to preserve type for extendErr
		return extendErr("failed to finalize contract: ", err)
	}
	defer h.managedUnlockStorageObligation(newSOID)
	err = modules.WriteNegotiationAcceptance(conn)
	if err != nil {
		return extendErr("failed to write acceptance: ", ErrorConnection(err.Error()))
	}
	// The host sends the transaction signatures to the renter, followed by the
	// revision signature. Negotiation is complete.
	err = encoding.WriteObject(conn, hostTxnSignatures)
	if err != nil {
		return extendErr("failed to write transaction signatures: ", ErrorConnection(err.Error()))
	}
	err = encoding.WriteObject(conn, hostRevisionSignature)
	if err != nil {
		return extendErr("failed to write revision signature: ", ErrorConnection(err.Error()))
	}
	return nil
}

// managedVerifyRenewedContract checks that the contract renewal matches the
// previous contract and makes all of the appropriate payments.
func (h *Host) managedVerifyRenewedContract(so storageObligation, txnSet []types.Transaction, renterPK crypto.PublicKey) error {
	// Check that the transaction set is not empty.
	if len(txnSet) < 1 {
		return extendErr("zero-length transaction set: ", errEmptyObject)
	}
	// Check that the transaction set has a file contract.
	if len(txnSet[len(txnSet)-1].FileContracts) < 1 {
		return extendErr("transaction without file contract: ", errEmptyObject)
	}

	h.mu.Lock()
	blockHeight := h.blockHeight
	externalSettings := h.externalSettings()
	internalSettings := h.settings
	lockedStorageCollateral := h.financialMetrics.LockedStorageCollateral
	publicKey := h.publicKey
	unlockHash := h.unlockHash
	h.mu.Unlock()
	fc := txnSet[len(txnSet)-1].FileContracts[0]

	// The file size and merkle root must match the file size and merkle root
	// from the previous file contract.
	if fc.FileSize != so.fileSize() {
		return errBadFileSize
	}
	if fc.FileMerkleRoot != so.merkleRoot() {
		return errBadFileMerkleRoot
	}
	// The WindowStart must be at least revisionSubmissionBuffer blocks into
	// the future.
	if fc.WindowStart <= blockHeight+revisionSubmissionBuffer {
		return errEarlyWindow
	}
	// WindowEnd must be at least settings.WindowSize blocks after WindowStart.
	if fc.WindowEnd < fc.WindowStart+externalSettings.WindowSize {
		return errSmallWindow
	}
	// WindowStart must not be more than settings.MaxDuration blocks into the
	// future.
	if fc.WindowStart > blockHeight+externalSettings.MaxDuration {
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

	// Check that the collateral does not exceed the maximum amount of
	// collateral allowed.
	expectedCollateral := renewContractCollateral(so, externalSettings, fc)
	if expectedCollateral.Cmp(externalSettings.MaxCollateral) > 0 {
		return errMaxCollateralReached
	}
	// Check that the host has enough room in the collateral budget to add this
	// collateral.
	if lockedStorageCollateral.Add(expectedCollateral).Cmp(internalSettings.CollateralBudget) > 0 {
		return errCollateralBudgetExceeded
	}
	// Check that the missed proof outputs contain enough money, and that the
	// void output contains enough money.
	basePrice := renewBasePrice(so, externalSettings, fc)
	baseCollateral := renewBaseCollateral(so, externalSettings, fc)
	if fc.ValidProofOutputs[1].Value.Cmp(basePrice.Add(baseCollateral)) < 0 {
		return errLowHostValidOutput
	}
	expectedHostMissedOutput := fc.ValidProofOutputs[1].Value.Sub(basePrice).Sub(baseCollateral)
	if fc.MissedProofOutputs[1].Value.Cmp(expectedHostMissedOutput) < 0 {
		return errLowHostMissedOutput
	}
	// Check that the void output has the correct value.
	expectedVoidOutput := basePrice.Add(baseCollateral)
	if fc.MissedProofOutputs[2].Value.Cmp(expectedVoidOutput) > 0 {
		return errLowVoidOutput
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
