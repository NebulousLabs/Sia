// host is an implementation of the host module, and is responsible for storing
// data and advertising available storage to the network.
package host

import (
	"errors"
	"net"
	"path/filepath"
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

	defaultTotalStorage = 10e9      // 10 GB.
	defaultMaxDuration  = 144 * 120 // 120 days.
	defaultWindowSize   = 144 * 2   // 2 days.
)

var (
	// defaultPrice defines the starting price for hosts selling storage. We
	// try to match a number that is both reasonably profitable and reasonably
	// competitive.
	defaultPrice = types.SiacoinPrecision.Div(types.NewCurrency64(4320e9)).Mul(types.NewCurrency64(100)) // 100 SC / GB / Month

	// defaultCollateral defines the amount of money that the host puts up as
	// collateral per-byte by default. Set to zero currently because neither of
	// the negotiation protocols have logic to deal with non-zero collateral.
	defaultCollateral = types.NewCurrency64(0)

	// errChangedUnlockHash is returned by SetSettings if the unlock hash has
	// changed, an illegal operation.
	errChangedUnlockHash = errors.New("cannot change the unlock hash in SetSettings")

	// Nil dependency errors.
	errNilCS     = errors.New("host cannot use a nil state")
	errNilTpool  = errors.New("host cannot use a nil transaction pool")
	errNilWallet = errors.New("host cannot use a nil wallet")
)

// A contractObligation tracks a file contract that the host is obligated to
// fulfill.
type contractObligation struct {
	ID              types.FileContractID
	FileContract    types.FileContract
	LastRevisionTxn types.Transaction
	Path            string // Where on disk the file is stored.

	// The mutex ensures that revisions are happening in serial. The actual
	// data under the obligations is being protected by the host's mutex.
	// Grabbing 'mu' is not sufficient to guarantee modification safety of the
	// struct, the host mutex must also be grabbed.
	mu sync.Mutex
}

// A Host contains all the fields necessary for storing files for clients and
// performing the storage proofs on the received files.
type Host struct {
	// Module dependencies.
	cs     modules.ConsensusSet
	tpool  modules.TransactionPool
	wallet modules.Wallet

	// Consensus Tracking
	blockHeight  types.BlockHeight
	recentChange modules.ConsensusChangeID

	// Host Identity
	netAddress modules.NetAddress
	publicKey  types.SiaPublicKey
	secretKey  crypto.SecretKey

	// File Management.
	obligationsByID     map[types.FileContractID]*contractObligation
	obligationsByHeight map[types.BlockHeight][]*contractObligation

	// Statistics
	profit         types.Currency
	fileCounter    int64
	spaceRemaining int64

	// Utilities.
	listener   net.Listener
	log        *persist.Logger
	mu         sync.RWMutex
	persistDir string
	settings   modules.HostSettings
}

// New returns an initialized Host.
func New(cs modules.ConsensusSet, tpool modules.TransactionPool, wallet modules.Wallet, address string, persistDir string) (*Host, error) {
	// Check that all the dependencies were provided.
	if cs == nil {
		return nil, errNilCS
	}
	if tpool == nil {
		return nil, errNilTpool
	}
	if wallet == nil {
		return nil, errNilWallet
	}

	// Create the host object.
	h := &Host{
		cs:     cs,
		tpool:  tpool,
		wallet: wallet,

		obligationsByID:     make(map[types.FileContractID]*contractObligation),
		obligationsByHeight: make(map[types.BlockHeight][]*contractObligation),

		persistDir: persistDir,
	}

	// Load all of the saved host state into the host.
	err := h.initPersist()
	if err != nil {
		return nil, err
	}

	// Initialize the logger.
	h.log, err = persist.NewLogger(filepath.Join(h.persistDir, logFile))
	if err != nil {
		return nil, err
	}

	// Subscribe to the consensus set.
	err = h.initConsensusSubscription()
	if err != nil {
		return nil, err
	}

	// Get the host established on the network.
	err = h.initNetworking(address)
	if err != nil {
		return nil, err
	}

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

// Close shuts down the host, preparing it for garbage collection.
func (h *Host) Close() error {
	// The order in which things are closed has been explicitly chosen to
	// minimize turbulence in the event of an error.

	// Save the latest host state.
	h.mu.Lock()
	err := h.save()
	h.mu.Unlock()
	if err != nil {
		return err
	}

	// Clean up networking processes.
	h.clearPort(h.netAddress.Port())
	err = h.listener.Close()
	if err != nil {
		return err
	}

	// Close the logger.
	err = h.log.Close()
	if err != nil {
		return err
	}

	h.cs.Unsubscribe(h)
	return nil
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
func (h *Host) SetSettings(settings modules.HostSettings) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Check that the unlock hash was not changed.
	if settings.UnlockHash != h.settings.UnlockHash {
		return errChangedUnlockHash
	}

	// Update the amount of space remaining to reflect the new volume of total
	// storage.
	h.spaceRemaining += settings.TotalStorage - h.settings.TotalStorage

	h.settings = settings
	return h.save()
}

// Settings returns the settings of a host.
func (h *Host) Settings() modules.HostSettings {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.settings
}
