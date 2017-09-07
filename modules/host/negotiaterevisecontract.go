package host

import (
	"fmt"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// managedRevisionIteration handles one iteration of the revision loop. As a
// performance optimization, multiple iterations of revisions are allowed to be
// made over the same connection.
func (h *Host) managedRevisionIteration(conn net.Conn, so *storageObligation, finalIter bool) error {
	// Read some variables from the host for use later in the function.
	h.mu.RLock()
	settings := h.externalSettings()
	secretKey := h.secretKey
	blockHeight := h.blockHeight
	h.mu.RUnlock()

	// Set the negotiation deadline.
	conn.SetDeadline(time.Now().Add(modules.NegotiateFileContractRevisionTime))

	// Get the signed revision
	var request modules.RevisionRequest
	err := encoding.ReadObject(conn, &request, modules.NegotiateMaxFileContractRevisionSize+modules.NegotiateMaxTransactionSignatureSize)
	if err != nil {
		return extendErr("unable to read revision modifications: ", ErrorConnection(err.Error()))
	}
	if request.Stop {
		return modules.ErrStopResponse // managedRPCReviseContract will catch this and exit gracefully
	}

	// Verify that the signature is valid and get the host's signature.
	txn, err := createRevisionSignature(request.Revision, request.Signature, secretKey, blockHeight)
	if err != nil {
		modules.WriteNegotiationRejection(conn, err) // Error is ignored so that the error type can be preserved in extendErr.
		return extendErr("could not create revision signature: ", err)
	}

	// Get the proposed actions
	// TODO change size. MaxReviseBatchSize is too much
	var actions []modules.RevisionAction
	err = encoding.ReadObject(conn, &actions, settings.MaxReviseBatchSize)

	// Send the settings to the renter. The host will keep going even if it is
	// not accepting contracts, because in this case the contract already
	// exists.
	err = h.managedRPCSettings(conn)
	if err != nil {
		return extendErr("RPCSettings failed: ", err)
	}

	// Reset deadline after managedRPCSettings shortened it
	// TODO: think about a better way to do that
	conn.SetDeadline(time.Now().Add(modules.NegotiateFileContractRevisionTime))

	// First read all of the modifications. Then make the modifications, but
	// with the ability to reverse them. Then verify the file contract revision
	// correctly accounts for the changes.
	var uploadRevenue types.Currency
	var downloadRevenue types.Currency
	var storageRevenue types.Currency

	var newCollateral types.Currency
	var sectorsRemoved []crypto.Hash
	var sectorsGained []crypto.Hash
	var gainedSectorData [][]byte
	var payload [][]byte
	var totalPayloadSize uint64
	var merkleRootChange = false
	err = func() error {
		for _, action := range actions {
			// Check that the index points to an existing sector root. If the type
			// is ActionInsert, we permit inserting at the end.
			if action.Type == modules.ActionInsert {
				if action.SectorIndex > uint64(len(so.SectorRoots)) {
					return errBadModificationIndex
				}
			} else if action.SectorIndex >= uint64(len(so.SectorRoots)) {
				return errBadModificationIndex
			}
			// Check that the total requested payload is not too large
			if totalPayloadSize > settings.MaxDownloadBatchSize {
				return extendErr("download iteration batch failed: ", errLargeDownloadBatch)
			}
			// If the Merkle root changed we need to verify it later
			if action.Type != modules.ActionDownload {
				merkleRootChange = true
			}

			// If the action requires additional data request it from the renter
			var data []byte
			if action.Type == modules.ActionInsert || action.Type == modules.ActionModify {
				if err := encoding.ReadObject(conn, &data, modules.SectorSize+8); err != nil {
					return extendErr("unable to read data for action: ", ErrorConnection(err.Error()))
				}
			}

			switch action.Type {
			case modules.ActionDelete:
				so.SectorRoots = append(so.SectorRoots[0:action.SectorIndex], so.SectorRoots[action.SectorIndex+1:]...)
				sectorsRemoved = append(sectorsRemoved, so.SectorRoots[action.SectorIndex])

			case modules.ActionInsert:
				// Check that the sector size is correct.
				if uint64(len(data)) != modules.SectorSize {
					return errBadSectorSize
				}

				// Update finances.
				blocksRemaining := so.proofDeadline() - blockHeight
				blockBytesCurrency := types.NewCurrency64(uint64(blocksRemaining)).Mul64(modules.SectorSize)
				uploadRevenue = uploadRevenue.Add(settings.UploadBandwidthPrice.Mul64(modules.SectorSize))
				storageRevenue = storageRevenue.Add(settings.StoragePrice.Mul(blockBytesCurrency))
				newCollateral = newCollateral.Add(settings.Collateral.Mul(blockBytesCurrency))

				// Insert the sector into the root list.
				newRoot := crypto.MerkleRoot(data)
				sectorsGained = append(sectorsGained, newRoot)
				gainedSectorData = append(gainedSectorData, data)
				so.SectorRoots = append(so.SectorRoots[:action.SectorIndex], append([]crypto.Hash{newRoot}, so.SectorRoots[action.SectorIndex:]...)...)

			case modules.ActionModify:
				// Check that the offset and length are okay. Length is already
				// known to be appropriately small, but the offset needs to be
				// checked for being appropriately small as well otherwise there is
				// a risk of overflow.
				if action.Offset > modules.SectorSize || action.Offset+uint64(len(data)) > modules.SectorSize {
					return errIllegalOffsetAndLength
				}

				// Get the data for the new sector.
				sector, err := h.ReadSector(so.SectorRoots[action.SectorIndex])
				if err != nil {
					return extendErr("could not read sector: ", ErrorInternal(err.Error()))
				}
				copy(sector[action.Offset:], data)

				// Update finances.
				uploadRevenue = uploadRevenue.Add(settings.UploadBandwidthPrice.Mul64(uint64(len(data))))

				// Update the sectors removed and gained to indicate that the old
				// sector has been replaced with a new sector.
				newRoot := crypto.MerkleRoot(sector)
				sectorsRemoved = append(sectorsRemoved, so.SectorRoots[action.SectorIndex])
				sectorsGained = append(sectorsGained, newRoot)
				gainedSectorData = append(gainedSectorData, sector)
				so.SectorRoots[action.SectorIndex] = newRoot

			case modules.ActionDownload:
				// Check that the length of each file is in-bounds, and that the total
				// size being requested is acceptable.
				if action.Length > modules.SectorSize || action.Offset+action.Length > modules.SectorSize {
					return extendErr("download iteration request failed: ", errRequestOutOfBounds)
				}
				totalPayloadSize += action.Length

				// Verify that the correct amount of money has been moved from the
				// renter's contract funds to the host's contract funds.
				downloadRevenue = downloadRevenue.Add(settings.DownloadBandwidthPrice.Mul64(action.Length))

				// Load the sectors and build the data payload.
				sectorData, err := h.ReadSector(action.MerkleRoot)
				if err != nil {
					return extendErr("failed to load sector: ", ErrorInternal(err.Error()))
				}
				payload = append(payload, sectorData[action.Offset:action.Offset+action.Length])

			default:
				return errUnknownModification
			}
			// If the action required additional data let the renter know that it was valid
			if action.Type == modules.ActionInsert || action.Type == modules.ActionModify {
				if err := modules.WriteNegotiationAcceptance(conn); err != nil {
					return extendErr("unable to send acceptance for data: ", ErrorConnection(err.Error()))
				}
			}
		}
		newRevenue := storageRevenue.Add(uploadRevenue).Add(downloadRevenue)
		return extendErr("unable to verify updated contract: ", verifyRevision(*so, request.Revision, blockHeight, newRevenue, newCollateral, merkleRootChange))
	}()
	if err != nil {
		modules.WriteNegotiationRejection(conn, err) // Error is ignored so that the error type can be preserved in extendErr.
		return extendErr("rejected proposed modifications: ", err)
	}

	so.PotentialStorageRevenue = so.PotentialStorageRevenue.Add(storageRevenue)
	so.RiskedCollateral = so.RiskedCollateral.Add(newCollateral)
	so.PotentialUploadRevenue = so.PotentialUploadRevenue.Add(uploadRevenue)
	so.PotentialDownloadRevenue = so.PotentialDownloadRevenue.Add(downloadRevenue)
	so.RevisionTransactionSet = []types.Transaction{txn}
	h.mu.Lock()
	err = h.modifyStorageObligation(*so, sectorsRemoved, sectorsGained, gainedSectorData)
	h.mu.Unlock()
	if err != nil {
		modules.WriteNegotiationRejection(conn, err) // Error is ignored so that the error type can be preserved in extendErr.
		return extendErr("could not modify storage obligation: ", ErrorInternal(err.Error()))
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
		return extendErr("iteration signal failed to send: ", ErrorConnection(err.Error()))
	}
	// Send the signature
	err = encoding.WriteObject(conn, txn.TransactionSignatures[1])
	if err != nil {
		return extendErr("failed to write revision signatures: ", ErrorConnection(err.Error()))
	}
	// if a payload was requested send it
	if len(payload) > 0 {
		err = encoding.WriteObject(conn, payload)
		if err != nil {
			return extendErr("failed to write payload: ", ErrorConnection(err.Error()))
		}
	}
	return nil
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
		return extendErr("RPCRecentRevision failed: ", err)
	}
	// The storage obligation is received with a lock on it. Defer a call to
	// unlock the storage obligation.
	defer func() {
		h.managedUnlockStorageObligation(so.id())
	}()

	// Begin the revision loop. The host will process revisions until a
	// timeout is reached, or until the renter sends a StopResponse.
	for timeoutReached := false; !timeoutReached; {
		timeoutReached = time.Since(startTime) > iteratedConnectionTime
		err := h.managedRevisionIteration(conn, &so, timeoutReached)
		if err == modules.ErrStopResponse {
			return nil
		} else if err != nil {
			return extendErr("revision iteration failed: ", err)
		}
	}
	return nil
}

// verifyRevision checks that the revision pays the host correctly, and that
// the revision does not attempt any malicious or unexpected changes.
func verifyRevision(so storageObligation, revision types.FileContractRevision, blockHeight types.BlockHeight, expectedExchange, expectedCollateral types.Currency, merkleRootChanged bool) error {
	// Check that the revision is well-formed.
	if len(revision.NewValidProofOutputs) != 2 || len(revision.NewMissedProofOutputs) != 3 {
		return errBadContractOutputCounts
	}

	// Check that the time to finalize and submit the file contract revision
	// has not already passed.
	if so.expiration()-revisionSubmissionBuffer <= blockHeight {
		return errLateRevision
	}

	oldFCR := so.RevisionTransactionSet[len(so.RevisionTransactionSet)-1].FileContractRevisions[0]

	// Check that all non-volatile fields are the same.
	if oldFCR.ParentID != revision.ParentID {
		return errBadContractParent
	}
	if oldFCR.UnlockConditions.UnlockHash() != revision.UnlockConditions.UnlockHash() {
		return errBadUnlockConditions
	}
	if oldFCR.NewRevisionNumber >= revision.NewRevisionNumber {
		return errBadRevisionNumber
	}
	if revision.NewFileSize != uint64(len(so.SectorRoots))*modules.SectorSize {
		return errBadFileSize
	}
	if oldFCR.NewWindowStart != revision.NewWindowStart {
		return errBadWindowStart
	}
	if oldFCR.NewWindowEnd != revision.NewWindowEnd {
		return errBadWindowEnd
	}
	if oldFCR.NewUnlockHash != revision.NewUnlockHash {
		return errBadUnlockHash
	}

	// Determine the amount that was transferred from the renter.
	if revision.NewValidProofOutputs[0].Value.Cmp(oldFCR.NewValidProofOutputs[0].Value) > 0 {
		return extendErr("renter increased its valid proof output: ", errHighRenterValidOutput)
	}
	fromRenter := oldFCR.NewValidProofOutputs[0].Value.Sub(revision.NewValidProofOutputs[0].Value)
	// Verify that enough money was transferred.
	if fromRenter.Cmp(expectedExchange) < 0 {
		s := fmt.Sprintf("expected at least %v to be exchanged, but %v was exchanged: ", expectedExchange, fromRenter)
		return extendErr(s, errHighRenterValidOutput)
	}

	// Determine the amount of money that was transferred to the host.
	if oldFCR.NewValidProofOutputs[1].Value.Cmp(revision.NewValidProofOutputs[1].Value) > 0 {
		return extendErr("host valid proof output was decreased: ", errLowHostValidOutput)
	}
	toHost := revision.NewValidProofOutputs[1].Value.Sub(oldFCR.NewValidProofOutputs[1].Value)
	// Verify that enough money was transferred.
	if !toHost.Equals(fromRenter) {
		s := fmt.Sprintf("expected exactly %v to be transferred to the host, but %v was transferred: ", fromRenter, toHost)
		return extendErr(s, errLowHostValidOutput)
	}

	// If the renter's valid proof output is larger than the renter's missed
	// proof output, the renter has incentive to see the host fail. Make sure
	// that this incentive is not present.
	if revision.NewValidProofOutputs[0].Value.Cmp(revision.NewMissedProofOutputs[0].Value) > 0 {
		return extendErr("renter has incentive to see host fail: ", errHighRenterMissedOutput)
	}

	// Check that the host is not going to be posting more collateral than is
	// expected. If the new misesd output is greater than the old one, the host
	// is actually posting negative collateral, which is fine.
	if revision.NewMissedProofOutputs[1].Value.Cmp(oldFCR.NewMissedProofOutputs[1].Value) <= 0 {
		collateral := oldFCR.NewMissedProofOutputs[1].Value.Sub(revision.NewMissedProofOutputs[1].Value)
		if collateral.Cmp(expectedCollateral) > 0 {
			s := fmt.Sprintf("host expected to post at most %v collateral, but contract has host posting %v: ", expectedCollateral, collateral)
			return extendErr(s, errLowHostMissedOutput)
		}
	}

	// Check that the revision count has increased.
	if revision.NewRevisionNumber <= oldFCR.NewRevisionNumber {
		return errBadRevisionNumber
	}

	// The Merkle root is checked last because it is the most expensive check.
	if !merkleRootChanged {
		// No need to check the Merkle root if request didn't change it
		return nil
	}
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
		return errBadFileMerkleRoot
	}

	return nil
}
