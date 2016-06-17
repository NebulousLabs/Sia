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
	// errBadModificationIndex is returned if the renter requests a change on a
	// sector root that is not in the file contract.
	errBadModificationIndex = errors.New("renter has made a modification that points to a nonexistent sector")

	// badSectorSize is returned if the renter provides a sector to be inserted
	// that is the wrong size.
	errBadSectorSize = errors.New("renter has provided an incorrectly sized sector")

	// errIllegalOffsetAndLength is returned if the renter tries perform a
	// modify operation that uses a troublesome combination of offset and
	// length.
	errIllegalOffsetAndLength = errors.New("renter is trying to do a modify with an illegal offset and length")

	// errLargeSector is returned if the renter sends a RevisionAction that has
	// data which creates a sector that is larger than what the host uses.
	errLargeSector = errors.New("renter has sent a sector that exceeds the host's sector size")

	// errLateRevision is returned if the renter is attempting to revise a
	// revision after the revision deadline. The host needs time to submit the
	// final revision to the blockchain to guarantee payment, and therefore
	// will not accept revisions once the window start is too close.
	errLateRevision = errors.New("renter is attempting to revise a revision which the host has closed")

	// errReviseBadCollateralDeduction is returned if a proposed file contrct
	// revision does not correctly deduct value from the host's missed proof
	// output - which is the host's collateral pool.
	errReviseBadCollateralDeduction = errors.New("proposed file contract revision does not correctly deduct from the host's collateral pool")

	// errReviseBadFileMerkleRoot is returned if the renter sends a file
	// contract revision with a Merkle root that does not match the changes
	// presented by the revision request.
	errReviseBadFileMerkleRoot = errors.New("proposed file contract revision has an incorrect Merkle merkle root")

	// errReviseBadHostValidOutput is returned if a proposed file contract
	// revision does not correctly add value to the host's valid proof outputs.
	errReviseBadHostValidOutput = errors.New("proposed file contract revision does not correctly add to the host's valid proof output")

	// errReviseBadNewFileSize is returned if a proposed file contract revision
	// does not have a file size which matches the revisions that have been
	// made.
	errReviseBadNewFileSize = errors.New("propsed file contract revision has a bad filesize")

	// errReviseBadNewWindowEnd is returned if a proposed file contract
	// revision does not have a window end which matches the original file
	// contract revision.
	errReviseBadNewWindowEnd = errors.New("proposed file contract revision has a bad window end")

	// errReviseBadNewWindowStart is returned if a proposed file contract
	// revision does not have a window start which matches the window start of
	// the original file contract.
	errReviseBadNewWindowStart = errors.New("propsed file contract revision has a bad window start")

	// errReviseBadParent is returned when a file contract revision is
	// presented which has a parent id that doesn't match the file contract
	// which is supposed to be getting revised.
	errReviseBadParent = errors.New("proposed file contract revision has the wrong parent id")

	// errReviseBadRenterValidOutput is returned if a propsed file contract
	// revision does not corectly deduct value from the renter's valid proof
	// output.
	errReviseBadRenterValidOutput = errors.New("proposed file contract revision does not correctly deduct from the renter's valid proof output")

	// errReviseBadRenterMissedOutput is returned if a proposed file contract
	// revision does not correctly deduct value from the renter's missed proof
	// output.
	errReviseBadRenterMissedOutput = errors.New("proposed file contract revision does not correctly deduct from the renter's missed proof output")

	// errReviseBadRevisionNumber is returned if a proposed file contract
	// revision does not have a revision number which is strictly greater than
	// the most recent revision number for the file contract being modified.
	errReviseBadRevisionNumber = errors.New("proposed file contract revision did not correctly increase the revision number")

	// errReviseBadUnlockConditions is returned when a file contract revision
	// has unlock conditions that do not match the file contract being revised.
	errReviseBadUnlockConditions = errors.New("propsed file contract revision appears to have the wrong unlock conditions")

	// errRevisionBadUnlockHash is returned if a proposed file contract
	// revision does not have an unlock hash which matches the unlock hash of
	// the previous file contract revision.
	errReviseBadUnlockHash = errors.New("proposed file contract revision has a bad new unlock hash")

	// errReviseBadVoidOutput is returned if a proposed file contract revision
	// does not correct add value to the void output to compensate for revenue
	// from the renter.
	errReviseBadVoidOutput = errors.New("proposed file contract revision does not correctly add to the host's void outputs")

	// errUnknownModification is returned if the host receives a modification
	// action from the renter that it does not understand.
	errUnknownModification = errors.New("renter is attempting an action that the host is not aware of")
)

// managedRevisionIteration handles one iteration of the revision loop. As a
// performance optimization, multiple iterations of revisions are allowed to be
// made over the same connection.
func (h *Host) managedRevisionIteration(conn net.Conn, so *storageObligation, finalIter bool) error {
	// Send the settings to the renter. The host will keep going even if it is
	// not accepting contracts, because in this case the contract already
	// exists.
	err := h.managedRPCSettings(conn)
	if err != nil {
		return err
	}

	// Set the negotiation deadline.
	conn.SetDeadline(time.Now().Add(modules.NegotiateFileContractRevisionTime))

	// The renter will either accept or reject the settings + revision
	// transaction. It may also return a stop response to indicate that it
	// wishes to terminate the revision loop.
	err = modules.ReadNegotiationAcceptance(conn)
	if err != nil {
		return err
	}

	// Read some variables from the host for use later in the function.
	h.mu.RLock()
	settings := h.settings
	secretKey := h.secretKey
	blockHeight := h.blockHeight
	h.mu.RUnlock()

	// The renter is going to send its intended modifications, followed by the
	// file contract revision that pays for them.
	var modifications []modules.RevisionAction
	var revision types.FileContractRevision
	err = encoding.ReadObject(conn, &modifications, settings.MaxReviseBatchSize)
	if err != nil {
		return err
	}
	err = encoding.ReadObject(conn, &revision, modules.NegotiateMaxFileContractRevisionSize)
	if err != nil {
		return err
	}

	// First read all of the modifications. Then make the modifications, but
	// with the ability to reverse them. Then verify the file contract revision
	// correctly accounts for the changes.
	var bandwidthRevenue types.Currency // Upload bandwidth.
	var storageRevenue types.Currency
	var newCollateral types.Currency
	var sectorsRemoved []crypto.Hash
	var sectorsGained []crypto.Hash
	var gainedSectorData [][]byte
	err = func() error {
		for _, modification := range modifications {
			// Check that the index points to an existing sector root. If the type
			// is ActionInsert, we permit inserting at the end.
			if modification.Type == modules.ActionInsert {
				if modification.SectorIndex > uint64(len(so.SectorRoots)) {
					return errBadModificationIndex
				}
			} else if modification.SectorIndex >= uint64(len(so.SectorRoots)) {
				return errBadModificationIndex
			}
			// Check that the data sent for the sector is not too large.
			if uint64(len(modification.Data)) > modules.SectorSize {
				return errLargeSector
			}

			switch modification.Type {
			case modules.ActionDelete:
				// There is no financial information to change, it is enough to
				// remove the sector.
				sectorsRemoved = append(sectorsRemoved, so.SectorRoots[modification.SectorIndex])
				so.SectorRoots = append(so.SectorRoots[0:modification.SectorIndex], so.SectorRoots[modification.SectorIndex+1:]...)
			case modules.ActionInsert:
				// Check that the sector size is correct.
				if uint64(len(modification.Data)) != modules.SectorSize {
					return errBadSectorSize
				}

				// Update finances.
				blocksRemaining := so.proofDeadline() - blockHeight
				blockBytesCurrency := types.NewCurrency64(uint64(blocksRemaining)).Mul64(modules.SectorSize)
				bandwidthRevenue = bandwidthRevenue.Add(settings.MinUploadBandwidthPrice.Mul64(modules.SectorSize))
				storageRevenue = storageRevenue.Add(settings.MinStoragePrice.Mul(blockBytesCurrency))
				newCollateral = newCollateral.Add(settings.Collateral.Mul(blockBytesCurrency))

				// Insert the sector into the root list.
				newRoot := crypto.MerkleRoot(modification.Data)
				sectorsGained = append(sectorsGained, newRoot)
				gainedSectorData = append(gainedSectorData, modification.Data)
				so.SectorRoots = append(so.SectorRoots[:modification.SectorIndex], append([]crypto.Hash{newRoot}, so.SectorRoots[modification.SectorIndex:]...)...)
			case modules.ActionModify:
				// Check that the offset and length are okay. Length is already
				// known to be appropriately small, but the offset needs to be
				// checked for being appropriately small as well otherwise there is
				// a risk of overflow.
				if modification.Offset > modules.SectorSize || modification.Offset+uint64(len(modification.Data)) > modules.SectorSize {
					return errIllegalOffsetAndLength
				}

				// Get the data for the new sector.
				sector, err := h.ReadSector(so.SectorRoots[modification.SectorIndex])
				if err != nil {
					return err
				}
				copy(sector[modification.Offset:], modification.Data)

				// Update finances.
				bandwidthRevenue = bandwidthRevenue.Add(settings.MinUploadBandwidthPrice.Mul64(uint64(len(modification.Data))))

				// Update the sectors removed and gained to indicate that the old
				// sector has been replaced with a new sector.
				newRoot := crypto.MerkleRoot(sector)
				sectorsRemoved = append(sectorsRemoved, so.SectorRoots[modification.SectorIndex])
				sectorsGained = append(sectorsGained, newRoot)
				gainedSectorData = append(gainedSectorData, sector)
				so.SectorRoots[modification.SectorIndex] = newRoot
			default:
				return errUnknownModification
			}
		}
		newRevenue := storageRevenue.Add(bandwidthRevenue)
		return verifyRevision(so, revision, blockHeight, newRevenue, newCollateral)
	}()
	if err != nil {
		return modules.WriteNegotiationRejection(conn, err)
	}
	// Revision is acceptable, write an acceptance string.
	err = modules.WriteNegotiationAcceptance(conn)
	if err != nil {
		return err
	}

	// Renter will send a transaction signature for the file contract revision.
	var renterSig types.TransactionSignature
	err = encoding.ReadObject(conn, &renterSig, modules.NegotiateMaxTransactionSignatureSize)
	if err != nil {
		return err
	}
	// Verify that the signature is valid and get the host's signature.
	txn, err := createRevisionSignature(revision, renterSig, secretKey, blockHeight)
	if err != nil {
		return modules.WriteNegotiationRejection(conn, err)
	}

	so.PotentialStorageRevenue = so.PotentialStorageRevenue.Add(storageRevenue)
	so.RiskedCollateral = so.RiskedCollateral.Add(newCollateral)
	so.PotentialUploadRevenue = so.PotentialUploadRevenue.Add(bandwidthRevenue)
	so.RevisionTransactionSet = []types.Transaction{txn}
	err = h.modifyStorageObligation(so, sectorsRemoved, sectorsGained, gainedSectorData)
	if err != nil {
		return modules.WriteNegotiationRejection(conn, err)
	}

	// Host will now send acceptance and its signature to the renter. This
	// iteration is complete. If the finalIter flag is set, StopResponse will
	// be sent instead. This indicates to the renter that the host wishes to
	// terminate the revision loop.
	if finalIter {
		err = modules.WriteNegotiationStop(conn)
	} else {
		err = modules.WriteNegotiationAcceptance(conn)
	}
	if err != nil {
		return err
	}
	return encoding.WriteObject(conn, txn.TransactionSignatures[1])
}

// managedRPCReviseContract accepts a request to revise an existing contract.
// Revisions can add sectors, delete sectors, and modify existing sectors.
func (h *Host) managedRPCReviseContract(conn net.Conn) error {
	// Set a preliminary deadline for receiving the storage obligation.
	startTime := time.Now()
	// Perform the file contract revision exchange, giving the renter the most
	// recent file contract revision and getting the storage obligation that
	// will be used to pay for the data.
	_, so, err := h.managedRPCRecentRevision(conn)
	if err != nil {
		return err
	}

	// Lock the storage obligation during the revision.
	h.mu.Lock()
	err = h.lockStorageObligation(so)
	h.mu.Unlock()
	if err != nil {
		return err
	}
	defer func() {
		h.mu.Lock()
		err = h.unlockStorageObligation(so)
		h.mu.Unlock()
		if err != nil {
			h.log.Critical(err)
		}
	}()

	// Begin the revision loop. The host will process revisions until a
	// timeout is reached, or until the renter sends a StopResponse.
	for timeoutReached := false; !timeoutReached; {
		timeoutReached = time.Since(startTime) > iteratedConnectionTime
		err := h.managedRevisionIteration(conn, so, timeoutReached)
		if err == modules.ErrStopResponse {
			return nil
		} else if err != nil {
			return err
		}
	}
	return nil
}

// verifyRevision checks that the revision pays the host correctly, and that
// the revision does not attempt any malicious or unexpected changes.
func verifyRevision(so *storageObligation, revision types.FileContractRevision, blockHeight types.BlockHeight, newRevenue, newCollateral types.Currency) error {
	// Check that the revision is well-formed.
	if len(revision.NewValidProofOutputs) != 2 || len(revision.NewMissedProofOutputs) != 3 {
		return errInsaneFileContractRevisionOutputCounts
	}

	// Check that the time to finalize and submit the file contract revision
	// has not already passed.
	if so.expiration()-revisionSubmissionBuffer <= blockHeight {
		return errLateRevision
	}

	oldFCR := so.RevisionTransactionSet[len(so.RevisionTransactionSet)-1].FileContractRevisions[0]

	// Check that all non-volatile fields are the same.
	if oldFCR.ParentID != revision.ParentID {
		return errReviseBadParent
	}
	if oldFCR.UnlockConditions.UnlockHash() != revision.UnlockConditions.UnlockHash() {
		return errReviseBadUnlockConditions
	}
	if oldFCR.NewRevisionNumber >= revision.NewRevisionNumber {
		return errReviseBadRevisionNumber
	}
	if revision.NewFileSize != uint64(len(so.SectorRoots))*modules.SectorSize {
		return errReviseBadNewFileSize
	}
	if oldFCR.NewWindowStart != revision.NewWindowStart {
		return errReviseBadNewWindowStart
	}
	if oldFCR.NewWindowEnd != revision.NewWindowEnd {
		return errReviseBadNewWindowEnd
	}
	if oldFCR.NewUnlockHash != revision.NewUnlockHash {
		return errReviseBadUnlockHash
	}

	// The new revenue comes out of the renter's valid outputs.
	if revision.NewValidProofOutputs[0].Value.Add(newRevenue).Cmp(oldFCR.NewValidProofOutputs[0].Value) > 0 {
		return errReviseBadRenterValidOutput
	}
	// The new revenue goes into the host's valid outputs.
	if oldFCR.NewValidProofOutputs[1].Value.Add(newRevenue).Cmp(revision.NewValidProofOutputs[1].Value) < 0 {
		return errReviseBadHostValidOutput
	}
	// The new revenue comes out of the renter's missed outputs.
	if revision.NewMissedProofOutputs[0].Value.Add(newRevenue).Cmp(oldFCR.NewMissedProofOutputs[0].Value) > 0 {
		return errReviseBadRenterMissedOutput
	}
	// The new collateral comes out of the host's missed outputs.
	if revision.NewMissedProofOutputs[1].Value.Add(newCollateral).Cmp(oldFCR.NewMissedProofOutputs[1].Value) > 0 {
		return errReviseBadCollateralDeduction
	}
	// The new collateral and new revenue goes into the host's void outputs.
	if oldFCR.NewMissedProofOutputs[2].Value.Add(newRevenue).Add(newCollateral).Cmp(revision.NewMissedProofOutputs[2].Value) < 0 {
		return errReviseBadVoidOutput
	}

	// The Merkle root is checked last because it is the most expensive check.
	log2SectorSize := uint64(0)
	for 1<<log2SectorSize < (modules.SectorSize / crypto.SegmentSize) {
		log2SectorSize++
	}
	ct := crypto.NewCachedTree(log2SectorSize)
	for _, root := range so.SectorRoots {
		ct.Push(root)
	}
	expectedMerkleRoot := ct.Root()
	if revision.NewFileMerkleRoot != expectedMerkleRoot {
		return errReviseBadFileMerkleRoot
	}

	return nil
}
