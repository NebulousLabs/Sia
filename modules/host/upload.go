package host

import (
	"errors"
	"io"
	"net"
	"os"
	"time"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
)

// RetrieveFile is an RPC that uploads a specified file to a client.
//
// Mutexes are applied carefully to avoid any disk intensive or network
// intensive operations. All necessary interaction with the host involves
// looking up the filepath of the file being requested. This is done all at
// once.
func (h *Host) RetrieveFile(conn net.Conn) (err error) {
	// Get the filename.
	var contractID consensus.FileContractID
	err = encoding.ReadObject(conn, &contractID, crypto.HashSize)
	if err != nil {
		return
	}

	// Verify the file exists, using a mutex while reading the host.
	h.mu.RLock()
	contractObligation, exists := h.obligationsByID[contractID]
	h.mu.RUnlock()
	if !exists {
		return errors.New("no record of that file")
	}

	// Open the file.
	file, err := os.Open(contractObligation.path)
	if err != nil {
		return
	}
	defer file.Close()
	info, _ := file.Stat()

	conn.SetDeadline(time.Now().Add(time.Duration(info.Size()) * 128 * time.Microsecond))

	// Transmit the file.
	_, err = io.Copy(conn, file)
	if err != nil {
		return
	}

	return
}
