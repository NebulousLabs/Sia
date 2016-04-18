package host

import (
	"net"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
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
	settings := h.settings
	h.mu.RUnlock()
	var requests []modules.DownloadAction
	err = encoding.ReadObject(conn, &requests, settings.MaxDownloadBatchSize)
	if err != nil {
		return err
	}
	// Read the file contract revision + signature that pay for the download
	// requests.
	var paymentRevision types.FileContractRevision
	var paymentSignature types.TransactionSignature
	err = encoding.ReadObject(conn, &paymentRevision, 16e3)
	if err != nil {
		return err
	}
	err = encoding.ReadObject(conn, &paymentSignature, 16e3)
	if err != nil {
		return err
	}

	// TODO: Verify that all of the sectors are known to the host, and pull out
	// the bytes that the renter has requested.

	// TODO: Verify that the payment is sufficient, and that the signauture is
	// valid.

	// TODO: sign the thing yourself.

	// Send the host signature back to the renter, followed by all of the data
	// that was requested.
	err = encoding.WriteObject(conn, hostSignature)
	if err != nil {
		return err
	}
	return encoding.WriteObject(conn, downloadPayload)
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
