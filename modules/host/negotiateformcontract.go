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
	// errBadContractUnlockHash is returned when the host receives a file
	// contract where it does not understand the unlock hash driving the
	// contract.
	errBadContractUnlockHash = errors.New("file contract has an unexpected unlock hash")

	// errBadFileSize is returned if a file contract is provided by the renter
	// which does not have the right file size.
	errBadFileSize = errors.New("new file contract does not have the right file size")

	// errBadFileMerkleRoot is returned if a file contract is provided by the
	// renter which does not have the right file size.
	errBadFileMerkleRoot = errors.New("new file contract does not have the right file Merkle root")

	// errBadPayoutsLen is returned if a new file contract is presented that
	// has the wrong number of valid or missed proof payouts.
	errBadPayoutsLen = errors.New("file contract has the wrong number of payouts - there should be two valid and three missed payouts")

	// errBadPayoutsAmounts is returned if a new file contract is presented that
	// does not pay the correct amount to the host - by default, the payouts
	// should be paying the contract price.
	errBadPayoutsAmounts = errors.New("file contract has payouts that do not correctly cover the contract price")

	// errBadPayoutsUnlockHashes is returned if a new file contract is
	// presented that does not make payments to the correct addresses.
	errBadPayoutsUnlockHashes = errors.New("file contract has payouts which pay to the wrong unlock hashes for the host")

	// errCollateralBudgetExceeded is returned if the host does not have enough
	// room in the collateral budget to accept a particular file contract.
	errCollateralBudgetExceeded = errors.New("host has reached its collateral budget and cannot accept the file contract")

	// errDurationTooLong is returned if the renter proposes a file contract
	// which is longer than the host's maximum duration.
	errDurationTooLong = errors.New("file contract has a duration which exceeds the duration permitted by the host")

	// errEmptyFileContractTransactionSet is returned if the renter provides a
	// nil file contract transaction set during file contract negotiation.
	errEmptyFileContractTransactionSet = errors.New("file contract transaction set is empty")

	// errLowHostPayout is returned if the host is not getting paid enough in
	// the file contract to cover the contract price.
	errLowHostPayout = errors.New("file contract payout does not cover the contract cost")

	// errLowFees is returned if a transaction set provided by the renter does
	// not have large enough transaction fees to have a reasonalbe chance at
	// making it onto the blockchain.
	errLowFees = errors.New("file contract proposal does not have enough transaction fees to be acceptable")

	// errMaxCollateralReached is returned if a file contract is provided which
	// would require the host to supply more collateral than the host allows
	// per file contract.
	errMaxCollateralReached = errors.New("file contract proposal expects the host to pay more than the maximum allowed collateral")

	// errNoFileContract is returned if a transaction set is sent that does not
	// have a file contract.
	errNoFileContract = errors.New("transaction set does not have a file contract")

	// errWindowSizeTooSmall is returned if a file contract has a window size
	// (defined by fc.WindowEnd - fc.WindowStart) which is too small to be
	// acceptable to the host - the host needs to submit its storage proof to
	// the blockchain inside of that window.
	errWindowSizeTooSmall = errors.New("file contract has a storage proof window which is not wide enough to match the host's requirements")

	// errWindowStartTooSoon is returned if the storage proof window for the
	// file contract opens too soon into the future - the host needs time to
	// submit the file contract and all revisions to the blockchain before the
	// storage proof window opens.
	errWindowStartTooSoon = errors.New("the storage proof window is opening too soon")
)

// contractCollateral returns the amount of collateral that the host is
// expected to add to the file contract based on the payout of the file
// contract and based on the host settings.
func contractCollateral(settings modules.HostInternalSettings, fc types.FileContract) types.Currency {
	return fc.ValidProofOutputs[1].Value.Sub(settings.MinContractPrice)
}

// managedAddCollateral adds the host's collateral to the file contract
// transaction set, returning the new inputs and outputs that get added to the
// transaction, as well as any new parents that get added to the transaction
// set. The builder that is used to add the collateral is also returned,
// because the new transaction has not yet been signed.
func (h *Host) managedAddCollateral(settings modules.HostInternalSettings, txnSet []types.Transaction) (builder modules.TransactionBuilder, newParents []types.Transaction, newInputs []types.SiacoinInput, newOutputs []types.SiacoinOutput, err error) {
	txn := txnSet[len(txnSet)-1]
	parents := txnSet[:len(txnSet)-1]
	fc := txn.FileContracts[0]
	hostPortion := contractCollateral(settings, fc)
	builder = h.wallet.RegisterTransaction(txn, parents)
	err = builder.FundSiacoins(hostPortion)
	if err != nil {
		builder.Drop()
		return nil, nil, nil, nil, err
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

// managedFinalizeContract will take a file contract, add the host's
// collateral, and then try submitting the file contract to the transaction
// pool. If there is no error, the completed transaction set will be returned
// to the caller.
func (h *Host) managedFinalizeContract(builder modules.TransactionBuilder, renterPK crypto.PublicKey, renterSignatures []types.TransactionSignature, renterRevisionSignature types.TransactionSignature) ([]types.TransactionSignature, types.TransactionSignature, error) {
	for _, sig := range renterSignatures {
		builder.AddTransactionSignature(sig)
	}
	fullTxnSet, err := builder.Sign(true)
	if err != nil {
		builder.Drop()
		return nil, types.TransactionSignature{}, err
	}

	// Verify that the signature for the revision from the renter is correct.
	h.mu.RLock()
	blockHeight := h.blockHeight
	hostSPK := h.publicKey
	hostSK := h.secretKey
	h.mu.RUnlock()
	contractTxn := fullTxnSet[len(fullTxnSet)-1]
	fc := contractTxn.FileContracts[0]
	noOpRevision := types.FileContractRevision{
		ParentID: contractTxn.FileContractID(0),
		UnlockConditions: types.UnlockConditions{
			PublicKeys: []types.SiaPublicKey{
				{
					Algorithm: types.SignatureEd25519,
					Key:       renterPK[:],
				},
				hostSPK,
			},
			SignaturesRequired: 2,
		},
		NewRevisionNumber: fc.RevisionNumber + 1,

		NewFileSize:           fc.FileSize,
		NewFileMerkleRoot:     fc.FileMerkleRoot,
		NewWindowStart:        fc.WindowStart,
		NewWindowEnd:          fc.WindowEnd,
		NewValidProofOutputs:  fc.ValidProofOutputs,
		NewMissedProofOutputs: fc.MissedProofOutputs,
		NewUnlockHash:         fc.UnlockHash,
	}
	// createRevisionSignature will also perform validation on the result,
	// returning an error if the renter.
	revisionTransaction, err := createRevisionSignature(noOpRevision, renterRevisionSignature, hostSK, blockHeight)
	if err != nil {
		return nil, types.TransactionSignature{}, err
	}

	// Create and add the storage obligation for this file contract.
	h.mu.Lock()
	defer h.mu.Unlock()
	fullTxn, _ := builder.View()
	hostPortion := contractCollateral(h.settings, fc)
	so := &storageObligation{
		ContractCost:     h.settings.MinContractPrice,
		LockedCollateral: hostPortion,

		OriginTransactionSet:   fullTxnSet,
		RevisionTransactionSet: []types.Transaction{revisionTransaction},
	}
	lockErr := h.lockStorageObligation(so)
	if lockErr != nil {
		return nil, types.TransactionSignature{}, lockErr
	}
	// addStorageObligation will submit the transaction to the transaction
	// pool, and will only do so if there was not some error in creating the
	// storage obligation.
	err = h.addStorageObligation(so)
	lockErr = h.unlockStorageObligation(so)
	if lockErr != nil {
		return nil, types.TransactionSignature{}, lockErr
	}
	if err != nil {
		// AcceptingContracts is set to false in the event of an error, because
		// it means that the host is having some type of disk error. Under
		// normal circumstances, adding a storage obligation should not cause
		// problems unexpectedly.
		h.log.Println(err)
		h.settings.AcceptingContracts = false
		builder.Drop()
		return nil, types.TransactionSignature{}, err
	}

	// Get the host's transaction signatures from the builder.
	var hostTxnSignatures []types.TransactionSignature
	_, _, _, txnSigIndices := builder.ViewAdded()
	for _, sigIndex := range txnSigIndices {
		hostTxnSignatures = append(hostTxnSignatures, fullTxn.TransactionSignatures[sigIndex])
	}
	return hostTxnSignatures, revisionTransaction.TransactionSignatures[1], nil
}

// managedRPCFormContract accepts a file contract from a renter, checks the
// file contract for compliance with the host settings, and then commits to the
// file contract, creating a storage obligation and submitting the contract to
// the blockchain.
func (h *Host) managedRPCFormContract(conn net.Conn) error {
	// Send the host settings to the renter.
	err := h.managedRPCSettings(conn)
	if err != nil {
		return err
	}
	// If the host is not accepting contracts, the connection can be closed.
	// The renter has been given enough information in the host settings to
	// understand that the connection is going to be closed.
	h.mu.RLock()
	settings := h.settings
	h.mu.RUnlock()
	if !settings.AcceptingContracts {
		return nil
	}

	// Extend the deadline to meet the rest of file contract negotiation.
	conn.SetDeadline(time.Now().Add(modules.NegotiateFileContractTime))

	// The renter will either accept or reject the host's settings.
	err = modules.ReadNegotiationAcceptance(conn)
	if err != nil {
		return err
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
		return err
	}
	err = encoding.ReadObject(conn, &renterPK, modules.NegotiateMaxSiaPubkeySize)
	if err != nil {
		return err
	}

	// The host verifies that the file contract coming over the wire is
	// acceptable.
	err = h.managedVerifyNewContract(txnSet, renterPK)
	if err != nil {
		// The incoming file contract is not acceptable to the host, indicate
		// why to the renter.
		return modules.WriteNegotiationRejection(conn, err)
	}
	// The host adds collateral to the transaction.
	txnBuilder, newParents, newInputs, newOutputs, err := h.managedAddCollateral(settings, txnSet)
	if err != nil {
		return modules.WriteNegotiationRejection(conn, err)
	}
	// The host indicates acceptance, and then sends any new parent
	// transactions, inputs and outputs that were added to the transaction.
	err = modules.WriteNegotiationAcceptance(conn)
	if err != nil {
		return err
	}
	err = encoding.WriteObject(conn, newParents)
	if err != nil {
		return err
	}
	err = encoding.WriteObject(conn, newInputs)
	if err != nil {
		return err
	}
	err = encoding.WriteObject(conn, newOutputs)
	if err != nil {
		return err
	}

	// The renter will now send a negotiation response, followed by transaction
	// signatures for the file contract transaction in the case of acceptance.
	// The transaction signatures will be followed by another transaction
	// siganture, to sign a no-op file contract revision.
	err = modules.ReadNegotiationAcceptance(conn)
	if err != nil {
		return err
	}
	var renterTxnSignatures []types.TransactionSignature
	var renterRevisionSignature types.TransactionSignature
	err = encoding.ReadObject(conn, &renterTxnSignatures, modules.NegotiateMaxTransactionSignaturesSize)
	if err != nil {
		return err
	}
	err = encoding.ReadObject(conn, &renterRevisionSignature, modules.NegotiateMaxTransactionSignatureSize)
	if err != nil {
		return err
	}

	// The host adds the renter transaction signatures, then signs the
	// transaction and submits it to the blockchain, creating a storage
	// obligation in the process. The host's part is done before anything is
	// written to the renter, but to give the renter confidence, the host will
	// send the signatures so that the renter can immediately have the
	// completed file contract.
	//
	// During finalization, the siganture for the revision is also checked, and
	// signatures for the revision transaction are created.
	hostTxnSignatures, hostRevisionSignature, err := h.managedFinalizeContract(txnBuilder, renterPK, renterTxnSignatures, renterRevisionSignature)
	if err != nil {
		// The incoming file contract is not acceptable to the host, indicate
		// why to the renter.
		return modules.WriteNegotiationRejection(conn, err)
	}
	err = modules.WriteNegotiationAcceptance(conn)
	if err != nil {
		return err
	}
	// The host sends the transaction signatures to the renter, followed by the
	// revision signature. Negotiation is complete.
	err = encoding.WriteObject(conn, hostTxnSignatures)
	if err != nil {
		return err
	}
	return encoding.WriteObject(conn, hostRevisionSignature)
}

// managedVerifyNewContract checks that an incoming file contract matches the host's
// expectations for a valid contract.
func (h *Host) managedVerifyNewContract(txnSet []types.Transaction, renterPK crypto.PublicKey) error {
	// Check that the transaction set is not empty.
	if len(txnSet) < 1 {
		return errEmptyFileContractTransactionSet
	}
	// Check that there is a file contract in the txnSet.
	if len(txnSet[len(txnSet)-1].FileContracts) < 1 {
		return errNoFileContract
	}

	h.mu.RLock()
	blockHeight := h.blockHeight
	lockedStorageCollateral := h.financialMetrics.LockedStorageCollateral
	publicKey := h.publicKey
	settings := h.settings
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
		return errWindowStartTooSoon
	}
	// WindowEnd must be at least settings.WindowSize blocks after
	// WindowStart.
	if fc.WindowEnd < fc.WindowStart+settings.WindowSize {
		return errWindowSizeTooSmall
	}
	// WindowEnd must not be more than settings.MaxDuration blocks into the
	// future.
	if fc.WindowStart > blockHeight+settings.MaxDuration {
		return errDurationTooLong
	}

	// ValidProofOutputs shoud have 2 outputs (renter + host) and missed
	// outputs should have 3 (renter + host + void)
	if len(fc.ValidProofOutputs) != 2 || len(fc.MissedProofOutputs) != 3 {
		return errBadPayoutsLen
	}
	// The unlock hashes of the valid and missed proof outputs for the host
	// must match the host's unlock hash. The third missed output should point
	// to the void.
	if fc.ValidProofOutputs[1].UnlockHash != unlockHash || fc.MissedProofOutputs[1].UnlockHash != unlockHash || fc.MissedProofOutputs[2].UnlockHash != (types.UnlockHash{}) {
		return errBadPayoutsUnlockHashes
	}
	// Check that the payouts for the valid proof outputs and the missed proof
	// outputs are the same - this is important because no data has been added
	// to the file contract yet.
	if fc.ValidProofOutputs[1].Value.Cmp(fc.MissedProofOutputs[1].Value) != 0 {
		return errBadPayoutsAmounts
	}
	// Check that there's enough payout for the host to cover at least the
	// contract price. This will prevent negative currency panics when working
	// with the collateral.
	if fc.ValidProofOutputs[1].Value.Cmp(settings.MinContractPrice) < 0 {
		return errLowHostPayout
	}
	// Check that the collateral does not exceed the maximum amount of
	// collateral allowed.
	expectedCollateral := contractCollateral(settings, fc)
	if expectedCollateral.Cmp(settings.MaxCollateral) > 0 {
		return errMaxCollateralReached
	}
	// Check that the host has enough room in the collateral budget to add this
	// collateral.
	if lockedStorageCollateral.Add(expectedCollateral).Cmp(settings.CollateralBudget) > 0 {
		return errCollateralBudgetExceeded
	}

	// The unlock hash for the file contract must match the unlock hash that
	// the host knows how to spend.
	expectedUH := types.UnlockConditions{
		PublicKeys: []types.SiaPublicKey{
			{
				Algorithm: types.SignatureEd25519,
				Key:       renterPK[:],
			},
			publicKey,
		},
		SignaturesRequired: 2,
	}.UnlockHash()
	if fc.UnlockHash != expectedUH {
		return errBadContractUnlockHash
	}

	// Check that the transaction set has enough fees on it to get into the
	// blockchain.
	setFee := modules.CalculateFee(txnSet)
	minFee, _ := h.tpool.FeeEstimation()
	if setFee.Cmp(minFee) < 0 {
		return errLowFees
	}
	return nil
}
