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
	tpool       modules.TransactionPool
	wallet      modules.Wallet
	latestBlock consensus.BlockID

	// our HostSettings, embedded for convenience
	modules.HostSettings

	hostDir        string
	spaceRemaining int64
	fileCounter    int

	contracts map[consensus.ContractID]contractObligation

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

	addr, _, err := wallet.CoinAddress()
	if err != nil {
		return
	}
	h = &Host{
		state:  state,
		wallet: wallet,

		// default host settings
		HostSettings: modules.HostSettings{
			MaxFilesize: 4 * 1000 * 1000,
			MaxDuration: 1008, // One week.
			MinWindow:   20,
			Price:       1,
			Burn:        1,
			CoinAddress: addr,
		},

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
	h.mu.RUnlock()
	if !exists {
		return errors.New("no record of that file")
	}

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

// SetConfig updates the host's internal HostSettings object. To modify
// a specific field, use a combination of Info and SetConfig
func (h *Host) SetConfig(settings modules.HostSettings) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.HostSettings = settings
}

// Settings is an RPC used to request the settings of a host.
func (h *Host) Settings() (modules.HostSettings, error) {
	// TODO: return an error if we haven't announced yet
	return h.HostSettings, nil
}

type HostInfo struct {
	modules.HostSettings

	StorageRemaining int64
	ContractCount    int
}

func (h *Host) Info() HostInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	info := HostInfo{
		HostSettings: h.HostSettings,

		StorageRemaining: h.spaceRemaining,
		ContractCount:    len(h.contracts),
	}
	return info
}
