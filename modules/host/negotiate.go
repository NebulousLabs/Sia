package host

import (
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	// errBadContractOutputCounts is returned if the presented file contract
	// revision has the wrong number of outputs for either the valid or the
	// missed proof outputs.
	errBadContractOutputCounts = ErrorCommunication("rejected for having an unexpected number of outputs")

	// errBadContractParent is returned when a file contract revision is
	// presented which has a parent id that doesn't match the file contract
	// which is supposed to be getting revised.
	errBadContractParent = ErrorCommunication("could not find contract's parent")

	// errBadFileMerkleRoot is returned if the renter incorrectly updates the
	// file merkle root during a file contract revision.
	errBadFileMerkleRoot = ErrorCommunication("rejected for bad file merkle root")

	// errBadFileSize is returned if the renter incorrectly download and
	// changes the file size during a file contract revision.
	errBadFileSize = ErrorCommunication("rejected for bad file size")

	// errBadModificationIndex is returned if the renter requests a change on a
	// sector root that is not in the file contract.
	errBadModificationIndex = ErrorCommunication("renter has made a modification that points to a nonexistent sector")

	// errBadParentID is returned if the renter incorrectly download and
	// provides the wrong parent id during a file contract revision.
	errBadParentID = ErrorCommunication("rejected for bad parent id")

	// errBadPayoutUnlockHashes is returned if the renter incorrectly sets the
	// payout unlock hashes during contract formation.
	errBadPayoutUnlockHashes = ErrorCommunication("rejected for bad unlock hashes in the payout")

	// errBadRevisionNumber number is returned if the renter incorrectly
	// download and does not increase the revision number during a file
	// contract revision.
	errBadRevisionNumber = ErrorCommunication("rejected for bad revision number")

	// errBadSectorSize is returned if the renter provides a sector to be
	// inserted that is the wrong size.
	errBadSectorSize = ErrorCommunication("renter has provided an incorrectly sized sector")

	// errBadUnlockConditions is returned if the renter incorrectly download
	// and does not provide the right unlock conditions in the payment
	// revision.
	errBadUnlockConditions = ErrorCommunication("rejected for bad unlock conditions")

	// errBadUnlockHash is returned if the renter incorrectly updates the
	// unlock hash during a file contract revision.
	errBadUnlockHash = ErrorCommunication("rejected for bad new unlock hash")

	// errBadWindowEnd is returned if the renter incorrectly download and
	// changes the window end during a file contract revision.
	errBadWindowEnd = ErrorCommunication("rejected for bad new window end")

	// errBadWindowStart is returned if the renter incorrectly updates the
	// window start during a file contract revision.
	errBadWindowStart = ErrorCommunication("rejected for bad new window start")

	// errEarlyWindow is returned if the file contract provided by the renter
	// has a storage proof window that is starting too near in the future.
	errEarlyWindow = ErrorCommunication("rejected for a window that starts too soon")

	// errEmptyObject is returned if the renter sends an empty or nil object
	// unexpectedly.
	errEmptyObject = ErrorCommunication("renter has unexpectedly send an empty/nil object")

	// errHighRenterMissedOutput is returned if the renter incorrectly download
	// and deducts an insufficient amount from the renter missed outputs during
	// a file contract revision.
	errHighRenterMissedOutput = ErrorCommunication("rejected for high paying renter missed output")

	// errHighRenterValidOutput is returned if the renter incorrectly download
	// and deducts an insufficient amount from the renter valid outputs during
	// a file contract revision.
	errHighRenterValidOutput = ErrorCommunication("rejected for high paying renter valid output")

	// errIllegalOffsetAndLength is returned if the renter tries perform a
	// modify operation that uses a troublesome combination of offset and
	// length.
	errIllegalOffsetAndLength = ErrorCommunication("renter is trying to do a modify with an illegal offset and length")

	// errLargeSector is returned if the renter sends a RevisionAction that has
	// data which creates a sector that is larger than what the host uses.
	errLargeSector = ErrorCommunication("renter has sent a sector that exceeds the host's sector size")

	// errLateRevision is returned if the renter is attempting to revise a
	// revision after the revision deadline. The host needs time to submit the
	// final revision to the blockchain to guarantee payment, and therefore
	// will not accept revisions once the window start is too close.
	errLateRevision = ErrorCommunication("renter is requesting revision after the revision deadline")

	// errLongDuration is returned if the renter proposes a file contract with
	// an experation that is too far into the future according to the host's
	// settings.
	errLongDuration = ErrorCommunication("renter proposed a file contract with a too-long duration")

	// errLowTransactionFees is returned if the renter provides a transaction
	// that the host does not feel is able to make it onto the blockchain.
	errLowTransactionFees = ErrorCommunication("rejected for including too few transaction fees")

	// errLowHostMissedOutput is returned if the renter incorrectly updates the
	// host missed proof output during a file contract revision.
	errLowHostMissedOutput = ErrorCommunication("rejected for low paying host missed output")

	// errLowHostValidOutput is returned if the renter incorrectly updates the
	// host valid proof output during a file contract revision.
	errLowHostValidOutput = ErrorCommunication("rejected for low paying host valid output")

	// errLowVoidOutput is returned if the renter has not allocated enough
	// funds to the void output.
	errLowVoidOutput = ErrorCommunication("rejected for low value void output")

	// errMismatchedHostPayouts is returned if the renter incorrectly sets the
	// host valid and missed payouts to different values during contract
	// formation.
	errMismatchedHostPayouts = ErrorCommunication("rejected because host valid and missed payouts are not the same value")

	// errSmallWindow is returned if the renter suggests a storage proof window
	// that is too small.
	errSmallWindow = ErrorCommunication("rejected for small window size")

	// errUnknownModification is returned if the host receives a modification
	// action from the renter that it does not understand.
	errUnknownModification = ErrorCommunication("renter is attempting an action that the host does not understand")
)

// createRevisionSignature creates a signature for a file contract revision
// that signs on the file contract revision. The renter should have already
// provided the signature. createRevisionSignature will check to make sure that
// the renter's signature is valid.
func createRevisionSignature(fcr types.FileContractRevision, renterSig types.TransactionSignature, secretKey crypto.SecretKey, blockHeight types.BlockHeight) (types.Transaction, error) {
	hostSig := types.TransactionSignature{
		ParentID:       crypto.Hash(fcr.ParentID),
		PublicKeyIndex: 1,
		CoveredFields: types.CoveredFields{
			FileContractRevisions: []uint64{0},
		},
	}
	txn := types.Transaction{
		FileContractRevisions: []types.FileContractRevision{fcr},
		TransactionSignatures: []types.TransactionSignature{renterSig, hostSig},
	}
	sigHash := txn.SigHash(1)
	encodedSig, err := crypto.SignHash(sigHash, secretKey)
	if err != nil {
		return types.Transaction{}, err
	}
	txn.TransactionSignatures[1].Signature = encodedSig[:]
	err = modules.VerifyFileContractRevisionTransactionSignatures(fcr, txn.TransactionSignatures, blockHeight)
	if err != nil {
		return types.Transaction{}, err
	}
	return txn, nil
}

// managedFinalizeContract will take a file contract, add the host's
// collateral, and then try submitting the file contract to the transaction
// pool. If there is no error, the completed transaction set will be returned
// to the caller.
func (h *Host) managedFinalizeContract(builder modules.TransactionBuilder, renterPK crypto.PublicKey, renterSignatures []types.TransactionSignature, renterRevisionSignature types.TransactionSignature, initialSectorRoots []crypto.Hash, hostCollateral, hostInitialRevenue, hostInitialRisk types.Currency) ([]types.TransactionSignature, types.TransactionSignature, types.FileContractID, error) {
	for _, sig := range renterSignatures {
		builder.AddTransactionSignature(sig)
	}
	fullTxnSet, err := builder.Sign(true)
	if err != nil {
		builder.Drop()
		return nil, types.TransactionSignature{}, types.FileContractID{}, err
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
				types.Ed25519PublicKey(renterPK),
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
	// returning an error if the renter provided an incorrect signature.
	revisionTransaction, err := createRevisionSignature(noOpRevision, renterRevisionSignature, hostSK, blockHeight)
	if err != nil {
		return nil, types.TransactionSignature{}, types.FileContractID{}, err
	}

	// Create and add the storage obligation for this file contract.
	fullTxn, _ := builder.View()
	so := storageObligation{
		SectorRoots: initialSectorRoots,

		ContractCost:            h.settings.MinContractPrice,
		LockedCollateral:        hostCollateral,
		PotentialStorageRevenue: hostInitialRevenue,
		RiskedCollateral:        hostInitialRisk,

		OriginTransactionSet:   fullTxnSet,
		RevisionTransactionSet: []types.Transaction{revisionTransaction},
	}

	// Get a lock on the storage obligation.
	lockErr := h.managedTryLockStorageObligation(so.id())
	if lockErr != nil {
		build.Critical("failed to get a lock on a brand new storage obligation")
		return nil, types.TransactionSignature{}, types.FileContractID{}, lockErr
	}
	defer func() {
		if err != nil {
			h.managedUnlockStorageObligation(so.id())
		}
	}()

	// addStorageObligation will submit the transaction to the transaction
	// pool, and will only do so if there was not some error in creating the
	// storage obligation. If the transaction pool returns a consensus
	// conflict, wait 30 seconds and try again.
	err = func() error {
		// Try adding the storage obligation. If there's an error, wait a few
		// seconds and try again. Eventually time out. It should be noted that
		// the storage obligation locking is both crappy and incomplete, and
		// that I'm not sure how this timeout plays with the overall host
		// timeouts.
		//
		// The storage obligation locks should occur at the highest level, not
		// just when the actual modification is happening.
		i := 0
		for {
			h.mu.Lock()
			err = h.addStorageObligation(so)
			h.mu.Unlock()
			if err == nil {
				return nil
			}
			if err != nil && i > 4 {
				h.log.Println(err)
				builder.Drop()
				return err
			}

			i++
			if build.Release == "standard" {
				time.Sleep(time.Second * 15)
			}
		}
	}()
	if err != nil {
		return nil, types.TransactionSignature{}, types.FileContractID{}, err
	}

	// Get the host's transaction signatures from the builder.
	var hostTxnSignatures []types.TransactionSignature
	_, _, _, txnSigIndices := builder.ViewAdded()
	for _, sigIndex := range txnSigIndices {
		hostTxnSignatures = append(hostTxnSignatures, fullTxn.TransactionSignatures[sigIndex])
	}
	return hostTxnSignatures, revisionTransaction.TransactionSignatures[1], so.id(), nil
}
