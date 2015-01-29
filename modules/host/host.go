package host

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/hash"
	"github.com/NebulousLabs/Sia/modules"
)

// TODO: Changing the host path should automatically move all of the files
// over.

const (
	StorageProofReorgDepth = 6 // How many blocks to wait before submitting a storage proof.
	maxContractLen         = 1 << 24
)

type contractObligation struct {
	filename string // Where on disk the file is stored.
}

type Host struct {
	state       *consensus.State
	wallet      modules.Wallet
	latestBlock consensus.BlockID

	hostDir string
	// announcement   modules.HostAnnouncement
	spaceRemaining int64
	fileCounter    int

	contracts map[consensus.ContractID]contractObligation // The string is filepath of the file being stored.

	mu sync.RWMutex
}

// New returns an initialized Host.
func New(state *consensus.State, wallet modules.Wallet) (h *Host, err error) {
	if wallet == nil {
		err = errors.New("host.New: cannot have nil wallet")
		return
	}
	if state == nil {
		err = errors.New("host.New: cannot have nil state")
		return
	}

	// addr, _, err := wallet.CoinAddress()
	if err != nil {
		return
	}
	h = &Host{
		state:  state,
		wallet: wallet,

		/*
			announcement: modules.HostAnnouncement{
				MaxFilesize: 4 * 1000 * 1000,
				MaxDuration: 1008, // One week.
				MinWindow:   20,
				Price:       1,
				Burn:        1,
				CoinAddress: addr,
			},
		*/

		contracts: make(map[consensus.ContractID]contractObligation),
	}

	return
}

// RetrieveFile is an RPC that uploads a specified file to a client.
//
// Mutexes are applied carefully to avoid any disk intensive or network
// intensive operations. All necessary interaction with the host involves
// looking up the filepath of the file being requested. This is done all at
// once.
//
// TODO: Move this function to a different file in the package?
func (h *Host) RetrieveFile(conn net.Conn) (err error) {
	// Get the filename.
	var contractID consensus.ContractID
	err = encoding.ReadObject(conn, &contractID, hash.HashSize)
	if err != nil {
		return
	}

	// Verify the file exists, using a mutex while reading the host.
	h.mu.RLock()
	contractObligation, exists := h.contracts[contractID]
	if !exists {
		h.mu.RUnlock()
		return errors.New("no record of that file")
	}
	h.mu.RUnlock()

	// Open the file.
	fullname := filepath.Join(h.hostDir, contractObligation.filename)
	file, err := os.Open(fullname)
	if err != nil {
		return
	}
	defer file.Close()

	// Transmit the file.
	_, err = io.Copy(conn, file)
	if err != nil {
		return
	}

	return
}

// TODO: Deprecate this function.
func (h *Host) NumContracts() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.contracts)
}
