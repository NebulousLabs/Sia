package host

import (
	"errors"
	"io"
	"net"
	"os"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// rpcDownload is an RPC that uploads requested segments of a file. After the
// RPC has been initiated, the host will read and process requests in a loop
// until the 'stop' signal is received or the connection times out.
func (h *Host) rpcDownload(conn net.Conn) error {
	// Read the contract ID.
	var contractID types.FileContractID
	err := encoding.ReadObject(conn, &contractID, crypto.HashSize)
	if err != nil {
		return err
	}

	// Verify the file exists, using a mutex while reading the host.
	h.mu.RLock()
	ob, exists := h.obligationsByID[contractID]
	if !exists {
		h.mu.RUnlock()
		return errors.New("no record of that file")
	}
	h.mu.RUnlock()

	// Open the file.
	file, err := os.Open(ob.Path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Process requests until 'stop' signal is received.
	var request modules.DownloadRequest
	for {
		if err := encoding.ReadObject(conn, &request, 16); err != nil {
			return err
		}
		// Check for termination signal.
		// TODO: perform other sanity checks on offset/length?
		if request.Length == 0 {
			break
		}

		conn.SetDeadline(time.Now().Add(5 * time.Minute)) // sufficient to transfer 4 MB over 100 kbps

		// Write segment to conn.
		segment := io.NewSectionReader(file, int64(request.Offset), int64(request.Length))
		_, err := io.Copy(conn, segment)
		if err != nil {
			return err
		}
	}
	return nil
}
