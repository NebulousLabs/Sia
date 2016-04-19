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
	// errLargeDownloadBatch is returned if the renter requests a download
	// batch that exceeds the maximum batch size that the host will
	// accomondate.
	errLargeDownloadBatch = errors.New("a batch of download requests has exceeded the maximum allowed download size")

	// errRequestOutOfBounds is returned when a download request is made which
	// asks for elements of a sector which do not exist.
	errRequestOutOfBounds = errors.New("a download request has been made that exceeds the sector boundaries")
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

	// Read a batch of download requests.
	h.mu.RLock()
	blockHeight := h.blockHeight
	secretKey := h.secretKey
	settings := h.settings
	h.mu.RUnlock()
	var requests []modules.DownloadAction
	err = encoding.ReadObject(conn, &requests, 50e3)
	if err != nil {
		return err
	}
	// Read the file contract revision that pays for the download requests.
	var paymentRevision types.FileContractRevision
	err = encoding.ReadObject(conn, &paymentRevision, 16e3)
	if err != nil {
		return err
	}

	// Verify that the request is acceptable, and then fetch all of the data
	// for the renter.
	var payload [][]byte
	var hostSignature types.TransactionSignature
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
		expectedTransfer := settings.MinimumDownloadBandwidthPrice.Mul(types.NewCurrency64(totalSize))
		existingRevision := so.RevisionTransactionSet[len(so.RevisionTransactionSet)-1].FileContractRevisions[0]
		// Check that NewValidProofOutputs[0] is deducted by expectedTransfer.
		if paymentRevision.NewValidProofOutputs[0].Value.Add(expectedTransfer).Cmp(existingRevision.NewValidProofOutputs[0].Value) != 0 {
			return errors.New("bad payments")
		}
		// Check that NewValidProofOutputs[1] is incremented by expectedTransfer.
		if existingRevision.NewValidProofOutputs[1].Value.Add(expectedTransfer).Cmp(paymentRevision.NewValidProofOutputs[1].Value) != 0 {
			return errors.New("bad payouts")
		}
		// Check that MissedProofOutputs[0] is decremented by expectedTransfer.
		if paymentRevision.NewMissedProofOutputs[0].Value.Add(expectedTransfer).Cmp(existingRevision.NewMissedProofOutputs[0].Value) != 0 {
			return errors.New("bad payments")
		}
		// Check that NewMissedProofOutputs[2] is incremented by expectedTransfer.
		if existingRevision.NewMissedProofOutputs[2].Value.Add(expectedTransfer).Cmp(paymentRevision.NewMissedProofOutputs[2].Value) != 0 {
			return errors.New("bad payouts")
		}
		// Check that the revision count has increased.
		if paymentRevision.NewRevisionNumber <= existingRevision.NewRevisionNumber {
			return errors.New("bad revision number")
		}

		// Check that no other fields have changed.
		if paymentRevision.ParentID != existingRevision.ParentID {
			return errors.New("unstable revision")
		}
		if paymentRevision.NewFileSize != existingRevision.NewFileSize {
			return errors.New("unstable revision")
		}
		if paymentRevision.NewFileMerkleRoot != existingRevision.NewFileMerkleRoot {
			return errors.New("unstable revision")
		}
		if paymentRevision.NewWindowStart != existingRevision.NewWindowStart {
			return errors.New("unstable revision")
		}
		if paymentRevision.NewWindowEnd != existingRevision.NewWindowEnd {
			return errors.New("unstable revision")
		}
		if paymentRevision.NewUnlockHash != existingRevision.NewUnlockHash {
			return errors.New("unstable revision")
		}
		if paymentRevision.NewMissedProofOutputs[1].Value.Cmp(existingRevision.NewMissedProofOutputs[1].Value) != 0 {
			return errors.New("unstable revision")
		}

		// Load the sectors and build the data payload.
		for _, request := range requests {
			sectorData, err := h.readSector(request.MerkleRoot)
			if err != nil {
				return err
			}
			payload = append(payload, sectorData[request.Offset:request.Offset+request.Length])
		}

		// Read the transaction signature from the renter.
		var paymentSignature types.TransactionSignature
		err = encoding.ReadObject(conn, &paymentSignature, 16e3)
		if err != nil {
			return err
		}
		// Create the host signature for the revision.
		cf := types.CoveredFields{
			FileContractRevisions: []uint64{0},
		}
		hostSignature = types.TransactionSignature{
			ParentID:       crypto.Hash(paymentRevision.ParentID),
			PublicKeyIndex: 1,
			CoveredFields:  cf,
		}
		txn := types.Transaction{
			FileContractRevisions: []types.FileContractRevision{paymentRevision},
			TransactionSignatures: []types.TransactionSignature{paymentSignature, hostSignature},
		}
		sigHash := txn.SigHash(1)
		encodedSig, err := crypto.SignHash(sigHash, secretKey)
		if err != nil {
			return err
		}
		txn.TransactionSignatures[1].Signature = encodedSig[:]

		// Verify that the renter signature is valid.
		err = txn.StandaloneValid(blockHeight)
		if err != nil {
			return modules.WriteNegotiationRejection(conn, err)
		}
		// Verify that the renter signature is covering the right fields.
		if paymentSignature.CoveredFields.WholeTransaction {
			return errors.New("renter cannot cover the whole transaction")
		}
		return nil
	}()
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
	err = encoding.WriteObject(conn, hostSignature)
	if err != nil {
		return err
	}
	return encoding.WriteObject(conn, payload)
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
	err = h.lockStorageObligation(so)
	if err != nil {
		return err
	}
	defer func() {
		err = h.unlockStorageObligation(so)
		if err != nil {
			h.log.Critical(err)
		}
	}()

	// Perform a loop that will allow downloads to happen until the maximum
	// time for a single connection has been reached.
	for time.Now().Before(startTime.Add(1200 * time.Second)) {
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
