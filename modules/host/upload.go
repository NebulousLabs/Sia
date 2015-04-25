package host

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// RetrieveFile is an RPC that uploads a specified file to a client.
//
// Mutexes are applied carefully to avoid any disk intensive or network
// intensive operations. All necessary interaction with the host involves
// looking up the filepath of the file being requested. This is done all at
// once.
func (h *Host) RetrieveFile(conn modules.NetConn) (err error) {
	// Get the filename.
	var contractID types.FileContractID
	err = conn.ReadObject(&contractID, crypto.HashSize)
	if err != nil {
		return
	}

	// Verify the file exists, using a mutex while reading the host.
	lockID := h.mu.RLock()
	contractObligation, exists := h.obligationsByID[contractID]
	h.mu.RUnlock(lockID)
	if !exists {
		return errors.New("no record of that file")
	}

	// Open the file.
	file, err := os.Open(filepath.Join(h.saveDir, contractObligation.Path))
	if err != nil {
		return
	}
	defer file.Close()

	// Transmit the file.
	_, err = io.CopyN(conn, file, int64(contractObligation.FileContract.FileSize))
	if err != nil {
		return
	}

	return
}
