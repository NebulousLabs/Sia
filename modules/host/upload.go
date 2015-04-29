package host

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

// rpcRetrieve is an RPC that uploads a specified file to a client.
//
// Mutexes are applied carefully to avoid locking during I/O. All necessary
// interaction with the host involves looking up the filepath of the file being
// requested. This is done all at once.
func (h *Host) rpcRetrieve(conn net.Conn) error {
	// Get the filename.
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
	path := filepath.Join(h.saveDir, contractObligation.Path)
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
