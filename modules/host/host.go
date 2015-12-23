// host is an implementation of the host module, and is responsible for storing
// data and advertising available storage to the network.
package host

import (
	"errors"
	"net"
	"sync"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// StorageProofReorgDepth states how many blocks to wait before submitting
	// a storage proof. This reduces the chance of needing to resubmit because
	// of a reorg.
	StorageProofReorgDepth = 10
	maxContractLen         = 1 << 16 // The maximum allowed size of a file contract coming in over the wire. This does not include the file.
)

var (
	// defaultPrice defines the starting price for hosts selling storage. We
	// try to match a number that is both reasonably profitable and reasonably
	// competitive.
	defaultPrice = types.SiacoinPrecision.Div(types.NewCurrency64(4320e9)).Mul(types.NewCurrency64(100)) // 100 SC / GB / Month
)

// A contractObligation tracks a file contract that the host is obligated to
// fulfill.
type contractObligation struct {
	ID              types.FileContractID
	FileContract    types.FileContract
	LastRevisionTxn types.Transaction
	Path            string // Where on disk the file is stored.

	// revisions must happen in serial
	mu sync.Mutex
}

// A Host contains all the fields necessary for storing files for clients and
// performing the storage proofs on the received files.
type Host struct {
	// Module dependencies.
	cs     modules.ConsensusSet
	tpool  modules.TransactionPool
	wallet modules.Wallet

	// Host Context.
	blockHeight types.BlockHeight
	netAddress  modules.NetAddress
	publicKey   types.SiaPublicKey
	secretKey   crypto.SecretKey
	settings    modules.HostSettings

	// File management.
	fileCounter         int64
	spaceRemaining      int64
	obligationsByID     map[types.FileContractID]*contractObligation
	obligationsByHeight map[types.BlockHeight][]*contractObligation

	// Statistics
	profit types.Currency

	// Utilities.
	listener   net.Listener
	log        *persist.Logger
	mu         sync.RWMutex
	persistDir string
}

// New returns an initialized Host.
func New(cs modules.ConsensusSet, tpool modules.TransactionPool, wallet modules.Wallet, address string, persistDir string) (*Host, error) {
	if cs == nil {
		return nil, errors.New("host cannot use a nil state")
	}
	if tpool == nil {
		return nil, errors.New("host cannot use a nil tpool")
	}
	if wallet == nil {
		return nil, errors.New("host cannot use a nil wallet")
	}

	h := &Host{
		cs:     cs,
		tpool:  tpool,
		wallet: wallet,

		persistDir: persistDir,

		obligationsByID:     make(map[types.FileContractID]*contractObligation),
		obligationsByHeight: make(map[types.BlockHeight][]*contractObligation),
	}

	// Load the old host data and initialize the logger.
	err := h.initPersist()
	if err != nil {
		return nil, err
	}

	// Get the host established on the network.
	err = h.initNetworking(address)
	if err != nil {
		return nil, err
	}

	h.cs.ConsensusSetSubscribe(h)

	return h, nil
}

// Capacity returns the amount of storage still available on the machine. The
// amount can be negative if the total capacity was reduced to below the active
// capacity.
func (h *Host) Capacity() int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.spaceRemaining
}

// Contracts returns the number of unresolved file contracts that the host is
// responsible for.
func (h *Host) Contracts() uint64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return uint64(len(h.obligationsByID))
}

// NetAddress returns the address at which the host can be reached.
func (h *Host) NetAddress() modules.NetAddress {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.netAddress
}

// Revenue returns the amount of revenue that the host has lined up, as well as
// the amount of revenue that the host has successfully captured.
func (h *Host) Revenue() (unresolved, resolved types.Currency) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, obligation := range h.obligationsByID {
		fc := obligation.FileContract
		unresolved = unresolved.Add(types.PostTax(h.blockHeight, fc.Payout))
	}
	return unresolved, h.profit
}

// SetSettings updates the host's internal HostSettings object.
func (h *Host) SetSettings(settings modules.HostSettings) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.spaceRemaining += settings.TotalStorage - h.settings.TotalStorage
	h.settings = settings
	h.save()
}

// Settings returns the settings of a host.
func (h *Host) Settings() modules.HostSettings {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.settings
}

// Close saves the state of the Gateway and stops its listener process.
func (h *Host) Close() error {
	h.mu.RLock()
	// save the latest host state
	if err := h.save(); err != nil {
		return err
	}
	h.mu.RUnlock()
	// clear the port mapping (no effect if UPnP not supported)
	h.clearPort(h.netAddress.Port())
	// shut down the listener
	return h.listener.Close()
}
