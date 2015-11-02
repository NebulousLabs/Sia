package host

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// rpcRetrieve is an RPC that uploads a specified file to a client.
//
// Mutexes are applied carefully to avoid locking during I/O. All necessary
// interaction with the host involves looking up the filepath of the file being
// requested. This is done all at once.
func (h *Host) rpcRetrieve(conn net.Conn) error {
	// Read the contract ID.
	var contractID types.FileContractID
	err := encoding.ReadObject(conn, &contractID, crypto.HashSize)
	if err != nil {
		return err
	}

	// Verify the file exists, using a mutex while reading the host.
	lockID := h.mu.RLock()
	contractObligation, exists := h.obligationsByID[contractID]
	if !exists {
		h.mu.RUnlock(lockID)
		return errors.New("no record of that file")
	}
	path := filepath.Join(h.persistDir, contractObligation.Path)
	h.mu.RUnlock(lockID)

	// Open the file.
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Transmit the file.
	_, err = io.CopyN(conn, file, int64(contractObligation.FileContract.FileSize))
	if err != nil {
		return err
	}

	return nil
}

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
	lockID := h.mu.RLock()
	co, exists := h.obligationsByID[contractID]
	if !exists {
		h.mu.RUnlock(lockID)
		return errors.New("no record of that file")
	}
	h.mu.RUnlock(lockID)

	// Open the file.
	file, err := os.Open(co.Path)
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
		// Write segment to conn.
		segment := io.NewSectionReader(file, int64(request.Offset), int64(request.Length))
		_, err := io.Copy(conn, segment)
		if err != nil {
			return err
		}
	}
	return nil
}
