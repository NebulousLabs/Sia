package host

import (
	"errors"
	"io"
	"net"
	"os"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/hash"
	"github.com/NebulousLabs/Sia/sia/components"
)

// TODO: Changing the host path should automatically move all of the files
// over.

const (
	StorageProofReorgDepth = 6 // How many blocks to wait before submitting a storage proof.
	maxContractLen         = 1 << 24
)

type contractObligation struct {
	inConsensus bool   // Whether the contract is recognized by the network.
	filename    string // Where on disk the file is stored.
}

type Host struct {
	state *consensus.State

	announcement   components.HostAnnouncement
	spaceRemaining int64
	wallet         components.Wallet

	transactionChan chan consensus.Transaction // TODO: Deprecated, subscription model should be implemented.

	hostDir     string
	fileCounter int
	contracts   map[consensus.ContractID]contractObligation // The string is filepath of the file being stored.

	mu sync.RWMutex
}

// New returns an initialized Host.
func New(state *consensus.State, wallet components.Wallet) (h *Host, err error) {
	if wallet == nil {
		err = errors.New("host.New: cannot have nil wallet")
		return
	}
	if state == nil {
		err = errors.New("host.New: cannot have nil state")
		return
	}

	h = &Host{
		state:     state,
		wallet:    wallet,
		contracts: make(map[consensus.ContractID]contractObligation),
	}

	// Subscribe to the state and begin listening for updates.
	// TODO: Get all changes/diffs from the genesis to current block in a way
	// that doesn't cause a race condition with the subscription.
	updateChan := state.ConsensusSubscribe()
	go h.consensusListen(updateChan)

	return
}

// UpdateHost changes the settings of the host to the input settings.
// SpaceRemaining will be changed accordingly, and will not return an error if
// space remaining goes negative.
func (h *Host) UpdateHost(update components.HostUpdate) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	storageDiff := update.Announcement.TotalStorage - h.announcement.TotalStorage
	h.spaceRemaining += storageDiff

	h.announcement = update.Announcement
	h.hostDir = update.HostDir
	h.transactionChan = update.TransactionChan
	h.wallet = update.Wallet
	return nil
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
	fullname := h.hostDir + contractObligation.filename
	h.mu.RUnlock()

	// Open the file.
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
