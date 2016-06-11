package host

import (
	"errors"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	// errDownloadBadHostValidOutputs is returned if the renter requests a
	// download and pays an insufficient amount to the host valid addresses.
	errDownloadBadHostValidOutputs = errors.New("download request rejected for bad host valid outputs")

	// errDownloadBadNewFileMerkleRoot is returned if the renter requests a
	// download and changes the file merkle root in the payment revision.
	errDownloadBadNewFileMerkleRoot = errors.New("download request rejected for bad file merkle root")

	// errDownloadBadNewFileSize is returned if the renter requests a download
	// and changes the file size in the payment revision.
	errDownloadBadNewFileSize = errors.New("download request rejected for bad file size")

	// errDownloadBadHostMissedOutputs is returned if the renter requests a
	// download and changes the host missed outputs in the payment revision.
	errDownloadBadHostMissedOutputs = errors.New("download request rejected for bad host missed outputs")

	// errDownloadBadNewWindowEnd is returned if the renter requests a download
	// and changes the window end in the payment revision.
	errDownloadBadNewWindowEnd = errors.New("download request rejected for bad new window end")

	// errDownloadBadNewWindowStart is returned if the renter requests a
	// download and changes the window start in the payment revision.
	errDownloadBadNewWindowStart = errors.New("download request rejected for bad new window start")

	// errDownloadBadNewUnlockHash is returned if the renter requests a
	// download and changes the unlock hash in the payment revision.
	errDownloadBadNewUnlockHash = errors.New("download request rejected for bad new unlock hash")

	// errDownloadBadParentID is returned if the renter requests a download and
	// provides the wrong parent id in the payment revision.
	errDownloadBadParentID = errors.New("download request rejected for bad parent id")

	// errDownloadBadRenterMissedOutputs is returned if the renter requests a
	// download and deducts an insufficient amount from the renter missed
	// outputs in the payment revision.
	errDownloadBadRenterMissedOutputs = errors.New("download request rejected for bad renter missed outputs")

	// errDownloadBadRenterValidOutputs is returned if the renter requests a
	// download and deducts an insufficient amount from the renter valid
	// outputs in the payment revision.
	errDownloadBadRenterValidOutputs = errors.New("download request rejected for bad renter valid outputs")

	// errDownloadBadRevision number is returned if the renter requests a
	// download and does not increase the revision number in the payment
	// revision.
	errDownloadBadRevisionNumber = errors.New("download request rejected for bad revision number")

	// errDownloadBadUnlockConditions is returned if the renter requests a
	// download and does not provide the right unlock conditions in the payment
	// revision.
	errDownloadBadUnlockConditions = errors.New("download request rejected for bad unlock conditions")

	// errDownloadBadVoidOutputs is returned if the renter requests a download
	// and does not add sufficient payment to the void outputs in the payment
	// revision.
	errDownloadBadVoidOutputs = errors.New("download request rejected for bad void outputs")

	// errLargeDownloadBatch is returned if the renter requests a download
	// batch that exceeds the maximum batch size that the host will
	// accomondate.
	errLargeDownloadBatch = errors.New("download request exceeded maximum batch size")

	// errRequestOutOfBounds is returned when a download request is made which
	// asks for elements of a sector which do not exist.
	errRequestOutOfBounds = errors.New("download request has invalid sector bounds")
)

// managedDownloadIteration is responsible for managing a single iteration of
// the download loop for RPCDownload.
func (h *Host) managedDownloadIteration(conn net.Conn, so *storageObligation) error {
	// Exchange settings with the renter.
	err := h.managedRPCSettings(conn)
	if err != nil {
		return err
	}

	// Extend the deadline for the download.
	conn.SetDeadline(time.Now().Add(modules.NegotiateDownloadTime))

	// The renter will either accept or reject the host's settings.
	err = modules.ReadNegotiationAcceptance(conn)
	if err != nil {
		return err
	}

	// Grab a set of variables that will be useful later in the function.
	h.mu.RLock()
	blockHeight := h.blockHeight
	secretKey := h.secretKey
	settings := h.settings
	h.mu.RUnlock()

	// Read the download requests, followed by the file contract revision that
	// pays for them.
	var requests []modules.DownloadAction
	var paymentRevision types.FileContractRevision
	err = encoding.ReadObject(conn, &requests, modules.NegotiateMaxDownloadActionRequestSize)
	if err != nil {
		return err
	}
	err = encoding.ReadObject(conn, &paymentRevision, modules.NegotiateMaxFileContractRevisionSize)
	if err != nil {
		return err
	}

	// Verify that the request is acceptable, and then fetch all of the data
	// for the renter.
	existingRevision := so.RevisionTransactionSet[len(so.RevisionTransactionSet)-1].FileContractRevisions[0]
	var payload [][]byte
	err = func() error {
		// Check that the length of each file is in-bounds, and that the total
		// size being requested is acceptable.
		var totalSize uint64
		for _, request := range requests {
			if request.Length > modules.SectorSize || request.Offset+request.Length > modules.SectorSize {
				return errRequestOutOfBounds
			}
			totalSize += request.Length
		}
		if totalSize > settings.MaxDownloadBatchSize {
			return errLargeDownloadBatch
		}

		// Verify that the correct amount of money has been moved from the
		// renter's contract funds to the host's contract funds.
		expectedTransfer := settings.MinDownloadBandwidthPrice.Mul64(totalSize)
		err = verifyPaymentRevision(existingRevision, paymentRevision, blockHeight, expectedTransfer)
		if err != nil {
			return err
		}

		// Load the sectors and build the data payload.
		for _, request := range requests {
			sectorData, err := h.ReadSector(request.MerkleRoot)
			if err != nil {
				return err
			}
			payload = append(payload, sectorData[request.Offset:request.Offset+request.Length])
		}
		return nil
	}()
	if err != nil {
		return modules.WriteNegotiationRejection(conn, err)
	}
	// Revision is acceptable, write acceptance.
	err = modules.WriteNegotiationAcceptance(conn)
	if err != nil {
		return err
	}

	// Renter will send a transaction siganture for the file contract revision.
	var renterSignature types.TransactionSignature
	err = encoding.ReadObject(conn, &renterSignature, modules.NegotiateMaxTransactionSignatureSize)
	if err != nil {
		return err
	}
	txn, err := createRevisionSignature(paymentRevision, renterSignature, secretKey, blockHeight)

	// Update the storage obligation.
	paymentTransfer := existingRevision.NewValidProofOutputs[0].Value.Sub(paymentRevision.NewValidProofOutputs[0].Value)
	so.PotentialDownloadRevenue = so.PotentialDownloadRevenue.Add(paymentTransfer)
	so.RevisionTransactionSet = []types.Transaction{{
		FileContractRevisions: []types.FileContractRevision{paymentRevision},
		TransactionSignatures: []types.TransactionSignature{renterSignature, txn.TransactionSignatures[1]},
	}}
	err = h.modifyStorageObligation(so, nil, nil, nil)
	if err != nil {
		return modules.WriteNegotiationRejection(conn, err)
	}

	// Write acceptance to the renter - the data request can be fulfilled by
	// the host, the payment is satisfactory, signature is correct. Then send
	// the host signature and all of the data.
	err = modules.WriteNegotiationAcceptance(conn)
	if err != nil {
		return err
	}
	err = encoding.WriteObject(conn, txn.TransactionSignatures[1])
	if err != nil {
		return err
	}
	return encoding.WriteObject(conn, payload)
}

// verifyPaymentRevision verifies that the revision being provided to pay for
// the data has transferred the expected amount of money from the renter to the
// host.
func verifyPaymentRevision(existingRevision, paymentRevision types.FileContractRevision, blockHeight types.BlockHeight, expectedTransfer types.Currency) error {
	// Check that the revision is well-formed.
	if len(paymentRevision.NewValidProofOutputs) != 2 || len(paymentRevision.NewMissedProofOutputs) != 3 {
		return errInsaneFileContractRevisionOutputCounts
	}

	// Check that the time to finalize and submit the file contract revision
	// has not already passed.
	if existingRevision.NewWindowStart-revisionSubmissionBuffer <= blockHeight {
		return errLateRevision
	}

	// The new revenue comes out of the renter's valid outputs.
	if paymentRevision.NewValidProofOutputs[0].Value.Add(expectedTransfer).Cmp(existingRevision.NewValidProofOutputs[0].Value) > 0 {
		return errDownloadBadRenterValidOutputs
	}
	// The new revenue goes into the host's valid outputs.
	if existingRevision.NewValidProofOutputs[1].Value.Add(expectedTransfer).Cmp(paymentRevision.NewValidProofOutputs[1].Value) < 0 {
		return errDownloadBadHostValidOutputs
	}
	// The new revenue comes out of the renter's missed outputs.
	if paymentRevision.NewMissedProofOutputs[0].Value.Add(expectedTransfer).Cmp(existingRevision.NewMissedProofOutputs[0].Value) > 0 {
		return errDownloadBadRenterMissedOutputs
	}
	// The new revenue goes into the void outputs.
	if existingRevision.NewMissedProofOutputs[2].Value.Add(expectedTransfer).Cmp(paymentRevision.NewMissedProofOutputs[2].Value) < 0 {
		return errDownloadBadVoidOutputs
	}
	// Check that the revision count has increased.
	if paymentRevision.NewRevisionNumber <= existingRevision.NewRevisionNumber {
		return errDownloadBadRevisionNumber
	}

	// Check that all of the non-volatile fields are the same.
	if paymentRevision.ParentID != existingRevision.ParentID {
		return errDownloadBadParentID
	}
	if paymentRevision.UnlockConditions.UnlockHash() != existingRevision.UnlockConditions.UnlockHash() {
		return errDownloadBadUnlockConditions
	}
	if paymentRevision.NewFileSize != existingRevision.NewFileSize {
		return errDownloadBadNewFileSize
	}
	if paymentRevision.NewFileMerkleRoot != existingRevision.NewFileMerkleRoot {
		return errDownloadBadNewFileMerkleRoot
	}
	if paymentRevision.NewWindowStart != existingRevision.NewWindowStart {
		return errDownloadBadNewWindowStart
	}
	if paymentRevision.NewWindowEnd != existingRevision.NewWindowEnd {
		return errDownloadBadNewWindowEnd
	}
	if paymentRevision.NewUnlockHash != existingRevision.NewUnlockHash {
		return errDownloadBadNewUnlockHash
	}
	if paymentRevision.NewMissedProofOutputs[1].Value.Cmp(existingRevision.NewMissedProofOutputs[1].Value) != 0 {
		return errDownloadBadHostMissedOutputs
	}
	return nil
}

// managedRPCDownload is responsible for handling an RPC request from the
// renter to download data.
func (h *Host) managedRPCDownload(conn net.Conn) error {
	// Get the start time to limit the length of the whole connection.
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

	// Perform a loop that will allow downloads to happen until the maximum
	// time for a single connection has been reached.
	for time.Now().Before(startTime.Add(iteratedConnectionTime)) {
		err := h.managedDownloadIteration(conn, so)
		if err == modules.ErrStopResponse {
			// The renter has indicated that it has finished downloading the
			// data, therefore there is no error. Return nil.
			return nil
		} else if err != nil {
			return err
		}
	}
	return nil
}
