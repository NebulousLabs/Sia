package host

// TODO: The revision transaction does need to be sent, because it needs to
// contain the transaction signatures. Furthermore, the 'WholeTransaction' flag
// on the transaction signatures needs to be set to false, something that the
// negotiation protocol needs to check.

// TODO: Since we're gathering untrusted input, need to check for both
// overflows and nil values.

import (
	"errors"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

var (
	// errBadModificationIndex is returned if the renter requests a change on a
	// sector root that is not in the file contract.
	errBadModificationIndex = errors.New("renter has made a modification that points to a nonexistant sector")

	// errBatchCountExceeded is returned if the renter tries to send a batch of
	// modifications that is larger than the maximum allowed batch.
	errBatchCountExceeded = errors.New("renter is trying to modify things greater than the max batch count")

	// errIllegalOffsetAndLength is returned if the renter tries perform a
	// modify operation that uses a troublesome combination of offset and
	// length.
	errIllegalOffsetAndLength = errors.New("renter is trying to do a modify with an illegal offset and length")

	// errUnknownModification is returned if the host receives a modification
	// action from the renter that it does not understand.
	errUnknownModification = errors.New("renter is attempting an action that the host is not aware of")
)

// managedRevisionIteration handles one iteration of the revision loop. As a
// performance optimization, multiple iterations of revisions are allowed to be
// made over the same connection.
func (h *Host) managedRevisionIteration(conn net.Conn, so *storageObligation) (bool, error) {
	// Set the negotiation deadline.
	conn.SetDeadline(time.Now().Add(modules.NegotiateFileContractRevisionTime))

	// Send the settings to the renter. The host will keep going even if it is
	// not accepting contracts, because in this case the contract already
	// exists.
	h.mu.RLock()
	settings := h.settings
	secretKey := h.secretKey
	blockHeight := h.blockHeight
	h.mu.RUnlock()
	err := crypto.WriteSignedObject(conn, settings, secretKey)
	if err != nil {
		return false, err
	}

	// Write the most recent file contract revision transaction.
	var revisionTxn types.Transaction
	if len(so.RevisionTransactionSet) > 0 {
		revisionTxn = so.RevisionTransactionSet[len(so.RevisionTransactionSet)-1]
	}
	err = encoding.WriteObject(conn, revisionTxn)
	if err != nil {
		return false, err
	}

	// The renter will either accept or reject the settings + revision
	// transaction.
	var acceptStr string
	err = encoding.ReadObject(conn, &acceptStr, modules.MaxErrorSize)
	if err != nil {
		return false, err
	}
	if acceptStr != modules.AcceptResponse {
		return false, errors.New(acceptStr)
	}

	// The renter is now going to send a batch of modifications followed by and
	// update file contract revision. Read the number of modifications being
	// sent by the renter.
	var modificationCount uint64
	err = encoding.ReadObject(conn, &modificationCount, 8)
	if err != nil {
		return false, err
	}
	if modificationCount > settings.MaxBatchSize {
		// The connection is closing unexpectedly on the renter, but the renter
		// has just received the settings and therefore the renter should know
		// better.
		return false, errBatchCountExceeded
	}

	// First read all of the modifications. Then make the modifications, but
	// with the ability to reverse them. Then verify the the file contract
	// revision that comes down the line.
	var bandwidthRevenue types.Currency
	var storageRevenue types.Currency
	var collateralRisked types.Currency
	var sectorsRemoved []crypto.Hash
	var sectorsGained []crypto.Hash
	var gainedSectorData [][]byte
	for i := uint64(0); i < modificationCount; i++ {
		// Read the type of modification that has been sent.
		var modificationType types.Specifier
		err = encoding.ReadObject(conn, &modificationType, uint64(len(modificationType)))
		if err != nil {
			return false, err
		}
		// Read the index of the sector that is going to be deleted.
		var index uint64
		err = encoding.ReadObject(conn, &index, 8)
		if err != nil {
			return false, err
		}
		// Check that the index points to an existing sector root.
		if uint64(len(so.SectorRoots)) <= index {
			return false, errBadModificationIndex
		}

		// Run a different codepath depending on the renter's selection.
		if modificationType == modules.ActionDelete {
			// There is no financial information to change, it is enough to
			// remove the sector.
			sectorsRemoved = append(sectorsRemoved, so.SectorRoots[index])
			so.SectorRoots = append(so.SectorRoots[0:index], so.SectorRoots[index+1:]...)
		} else if modificationType == modules.ActionInsert {
			// Download the sector.
			var sector []byte
			err = encoding.ReadObject(conn, &sector, modules.SectorSize+8)
			if err != nil {
				return false, err
			}

			// Update finances.
			blocksRemaining := so.proofDeadline() - blockHeight
			blockBytesCurrency := types.NewCurrency64(uint64(blocksRemaining)).Mul(types.NewCurrency64(modules.SectorSize))
			bandwidthRevenue = bandwidthRevenue.Add(settings.MinimumUploadBandwidthPrice.Mul(types.NewCurrency64(modules.SectorSize)))
			storageRevenue = storageRevenue.Add(settings.MinimumStoragePrice.Mul(blockBytesCurrency))
			collateralRisked = collateralRisked.Add(settings.Collateral.Mul(blockBytesCurrency))

			// Insert the sector into the root list.
			newRoot := crypto.MerkleRoot(sector[:])
			sectorsGained = append(sectorsGained, newRoot)
			gainedSectorData = append(gainedSectorData, sector[:])
			so.SectorRoots = append(so.SectorRoots[:index], append([]crypto.Hash{newRoot}, so.SectorRoots[index:]...)...)
		} else if modificationType == modules.ActionModify {
			// Download the sector.
			var offset uint64
			var length uint64
			err = encoding.ReadObject(conn, &offset, 8)
			if err != nil {
				return false, err
			}
			err = encoding.ReadObject(conn, &length, 8)
			if err != nil {
				return false, err
			}
			// Have to check all three cases, otherwise an attacker could abuse
			// overflows.
			if offset > modules.SectorSize || length > modules.SectorSize || offset+length > modules.SectorSize {
				return false, errIllegalOffsetAndLength
			}
			var sectorModifications []byte
			err = encoding.ReadObject(conn, &sectorModifications, length+8)
			if err != nil {
				return false, err
			}

			// Get the data for the new sector.
			sector, err := h.readSector(so.SectorRoots[index])
			if err != nil {
				return false, err
			}
			for i := offset; i < uint64(len(sectorModifications)); i++ {
				sector[i] = sectorModifications[i-offset]
			}

			// Update finances.
			bandwidthRevenue = bandwidthRevenue.Add(settings.MinimumUploadBandwidthPrice.Mul(types.NewCurrency64(modules.SectorSize)))

			// Update the sectors removed and gained to indicate that the old
			// sector has been replaced with a new sector.
			newRoot := crypto.MerkleRoot(sector[:])
			sectorsRemoved = append(sectorsRemoved, so.SectorRoots[index])
			sectorsGained = append(sectorsGained, newRoot)
			gainedSectorData = append(gainedSectorData, sector[:])
			so.SectorRoots[index] = newRoot
		} else {
			return false, errUnknownModification
		}
	}

	// Read the file contract revision and check whether it's acceptable.
	var revision types.FileContractRevision
	err = encoding.ReadObject(conn, &revision, 16e3)
	if err != nil {
		return false, err
	}
	err = verifyRevision(so, revision, storageRevenue, bandwidthRevenue, collateralRisked)
	if err != nil {
		return false, rejectNegotiation(conn, err)
	}

	// Revision is acceptable, write an acceptance string.
	err = encoding.WriteObject(conn, modules.AcceptResponse)
	if err != nil {
		return false, err
	}

	// Renter will now send the transaction signatures for the file contract,
	// followed by an indication of whether another iteration is preferred.
	var renterSig types.TransactionSignature
	var another bool
	err = encoding.ReadObject(conn, &renterSig, 16e3)
	if err != nil {
		return false, err
	}
	err = encoding.ReadObject(conn, &another, 16)
	if err != nil {
		return false, err
	}

	// Create the signatures for a transaction that contains only the file
	// contract revision and the renter signatures.
	// Create the CoveredFields for the signature.
	cf := types.CoveredFields{
		FileContractRevisions: []uint64{0},
		TransactionSignatures: []uint64{0},
	}
	hostTxnSig := types.TransactionSignature{
		ParentID:       crypto.Hash(revision.ParentID),
		PublicKeyIndex: 1,
		CoveredFields:  cf,
	}
	txn := types.Transaction{
		FileContractRevisions: []types.FileContractRevision{revision},
		TransactionSignatures: []types.TransactionSignature{renterSig, hostTxnSig},
	}
	sigHash := txn.SigHash(1)
	encodedSig, err := crypto.SignHash(sigHash, secretKey)
	if err != nil {
		return false, err
	}
	txn.TransactionSignatures[1].Signature = encodedSig[:]

	// Host will verify the transaction StandaloneValid is enough. If valid,
	// the host will update and submit the storage obligation.
	err = txn.StandaloneValid(blockHeight)
	if err != nil {
		return false, err
	}
	so.AnticipatedRevenue = so.AnticipatedRevenue.Add(storageRevenue)
	so.ConfirmedRevenue = so.ConfirmedRevenue.Add(bandwidthRevenue)
	so.RiskedCollateral = so.RiskedCollateral.Add(collateralRisked)
	err = h.modifyStorageObligation(so, sectorsRemoved, sectorsGained, gainedSectorData)
	if err != nil {
		return false, err
	}

	// Host will now send the signatures to the renter. This iteration is
	// complete.
	return another, encoding.WriteObject(conn, txn.TransactionSignatures[1])
}

// managedRPCReviseContract accepts a request to revise an existing contract.
// Revisions can add sectors, delete sectors, and modify existing sectors.
func (h *Host) managedRPCReviseContract(conn net.Conn) error {
	// Set a preliminary deadline for receiving the storage obligation.
	startTime := time.Now()
	conn.SetDeadline(time.Now().Add(modules.NegotiateFileContractRevisionTime))

	// Read the file contract id from the renter.
	var fcid types.FileContractID
	err := encoding.ReadObject(conn, &fcid, uint64(len(fcid)))
	if err != nil {
		return err
	}

	// Get and then lock the storage obligation.
	var so *storageObligation
	err = h.db.Update(func(tx *bolt.Tx) error {
		fso, innerErr := getStorageObligation(tx, fcid)
		so = &fso
		return innerErr
	})
	if err != nil {
		return err
	}
	err = h.lockStorageObligation(so)
	if err != nil {
		return err
	}
	defer h.unlockStorageObligation(so)

	// Indicate that the host is accepting the revision request.
	err = encoding.WriteObject(conn, modules.AcceptResponse)
	if err != nil {
		return err
	}

	// Upon connection, begin the revision loop.
	for time.Now().Before(startTime.Add(1200 * time.Second)) {
		another, err := h.managedRevisionIteration(conn, so)
		if err != nil {
			return err
		}
		// If the renter is not asking for another iteration, terminate the
		// connection.
		if !another {
			return nil
		}
	}
	return nil
}

// verifyRevision checks that the revision
//
// TODO: Finish implementation
func verifyRevision(so *storageObligation, revision types.FileContractRevision, storageRevenue, bandwidthRevenue, collateralRisked types.Currency) error {
	// Check that all non-volatile fields are the same.

	// Check that the root hash and the file size match the updated sector
	// roots.

	// Check that the payments have updated to reflect the new revenues.

	// Check that the revision number has increased.

	// Check any other thing that needs to be checked.
	return nil
}
