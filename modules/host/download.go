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

const (
	// tolerableDownloadSize specifies the maximum size that can be downloaded
	// in a single request. 64MB is chosen because most requests should be at
	// 4MB exactly and connections can be unstable beyond 200MB.
	tolerableDownloadSize = 1 << 26
)

// rpcDownload is an RPC that uploads requested segments of a file. After the
// RPC has been initiated, the host will read and process requests in a loop
// until the 'stop' signal is received or the connection times out.
//
// TODO: There is no lock obtained on the obligation, which means that a
// revision could modify the file at the same time that it is being read from
// disk.
func (h *Host) managedRPCDownload(conn net.Conn) error {
	// Read the contract ID.
	var contractID types.FileContractID
	err := encoding.ReadObject(conn, &contractID, crypto.HashSize)
	if err != nil {
		return err
	}

	// Verify the file exists, using a mutex while reading the host.
	h.mu.RLock()
	ob, exists := h.obligationsByID[contractID]
	h.mu.RUnlock()
	if !exists {
		return errors.New("no record of that file")
	}

	// Open the file.
	file, err := os.Open(ob.Path)
	if err != nil {
		return err
	}
	defer file.Close()
	fi, err := file.Stat()
	if err != nil {
		return err
	}

	// Process requests until 'stop' signal is received.
	var request modules.DownloadRequest
	for {
		if err := encoding.ReadObject(conn, &request, 16); err != nil {
			return err
		}

		// Check for termination signal.
		if request.Length == 0 {
			break
		}

		// Check for sane request parameters.
		if request.Length+request.Offset > uint64(fi.Size()) {
			return errors.New("request exceeds file bounds")
		}
		if request.Length > tolerableDownloadSize {
			return errors.New("cannot download provided length")
		}

		// Write segment to conn.
		err := conn.SetDeadline(time.Now().Add(5 * time.Minute)) // sufficient to transfer 4 MB over 100 kbps
		if err != nil {
			return err // TODO: Is this the correct way to handle this error?
		}
		segment := io.NewSectionReader(file, int64(request.Offset), int64(request.Length))
		_, err = io.Copy(conn, segment)
		if err != nil {
			return err
		}
	}
	return nil
}
