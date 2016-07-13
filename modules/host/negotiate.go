package host

import (
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
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
func (h *Host) managedFinalizeContract(builder modules.TransactionBuilder, renterPK crypto.PublicKey, renterSignatures []types.TransactionSignature, renterRevisionSignature types.TransactionSignature, initialSectorRoots []crypto.Hash, hostCollateral, hostInitialRevenue, hostInitialRisk types.Currency) ([]types.TransactionSignature, types.TransactionSignature, error) {
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
	// returning an error if the renter provided an incorrect signature.
	revisionTransaction, err := createRevisionSignature(noOpRevision, renterRevisionSignature, hostSK, blockHeight)
	if err != nil {
		return nil, types.TransactionSignature{}, err
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
	h.mu.Lock()
	lockErr := h.tryLockStorageObligation(so.id())
	h.mu.Unlock()
	if lockErr != nil {
		return nil, types.TransactionSignature{}, lockErr
	}

	// addStorageObligation will submit the transaction to the transaction
	// pool, and will only do so if there was not some error in creating the
	// storage obligation. If the transaction pool returns a consensus
	// conflict, wait 30 seconds and try again.
	err = func() error {
		// Unlock the storage obligation when finished.
		defer h.unlockStorageObligation(so.id())

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
