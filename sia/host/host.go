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

const (
	StorageProofReorgDepth = 6 // How many blocks to wait before submitting a storage proof.
	maxContractLen         = 1 << 24
)

type Host struct {
	announcement   components.HostAnnouncement
	spaceRemaining int64
	wallet         components.Wallet
	state          components.ReadOnlyState

	height          consensus.BlockHeight      // Current height of the state.
	transactionChan chan consensus.Transaction // Can send channels to the state.

	hostDir     string
	fileCounter int
	files       map[hash.Hash]string

	forwardContracts  map[consensus.BlockHeight][]ContractEntry
	backwardContracts map[consensus.BlockHeight][]ContractEntry

	rwLock sync.RWMutex
}

// New returns an initialized Host.
func New() (h *Host) {
	return &Host{
		files:             make(map[hash.Hash]string),
		forwardContracts:  make(map[consensus.BlockHeight][]ContractEntry),
		backwardContracts: make(map[consensus.BlockHeight][]ContractEntry),
	}
}

// UpdateHost changes the settings of the host to the input settings.
// SpaceRemaining will be changed accordingly, and will not return an error if
// space remaining goes negative.
func (h *Host) UpdateHost(update components.HostUpdate) error {
	h.lock()
	defer h.unlock()

	storageDiff := update.Announcement.TotalStorage - h.announcement.TotalStorage
	h.spaceRemaining += storageDiff

	h.announcement = update.Announcement
	h.height = update.Height
	h.hostDir = update.HostDir
	h.state = update.State
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
func (h *Host) RetrieveFile(conn net.Conn) (err error) {
	// Get the filename.
	var merkle hash.Hash
	err = encoding.ReadObject(conn, &merkle, hash.HashSize)
	if err != nil {
		return
	}

	// Verify the file exists, using a mutex while reading the host.
	h.rLock()
	filename, exists := h.files[merkle]
	fullname := h.hostDir + filename
	h.rUnlock()
	if !exists {
		return errors.New("no record of that file")
	}

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
