// Package host is an implementation of the host module, and is responsible for
// storing data and advertising available storage to the network.
package host

import (
	"errors"
	"net"
	"sync"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

const (
	maxContractLen      = 1 << 16   // The maximum allowed size of a file contract coming in over the wire. This does not include the file.
	defaultTotalStorage = 10e9      // 10 GB.
	defaultMaxDuration  = 144 * 120 // 120 days.
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

	// defaultWindowSize is the size of the proof of storage window requested
	// by the host. The host will not delete any obligations until the window
	// has closed and buried under several confirmations.
	defaultWindowSize = func() types.BlockHeight {
		if build.Release == "testing" {
			return 5
		}
		if build.Release == "standard" {
			return 36
		}
		if build.Release == "dev" {
			return 36
		}
		panic("unrecognized release constant in host")
	}()

	// errChangedUnlockHash is returned by SetSettings if the unlock hash has
	// changed, an illegal operation.
	errChangedUnlockHash = errors.New("cannot change the unlock hash in SetSettings")

	// errHostClosed gets returned when a call is rejected due to the host
	// having been closed.
	errHostClosed = errors.New("call is disabled because the host is closed")

	// Nil dependency errors.
	errNilCS     = errors.New("host cannot use a nil state")
	errNilTpool  = errors.New("host cannot use a nil transaction pool")
	errNilWallet = errors.New("host cannot use a nil wallet")
)

// A Host contains all the fields necessary for storing files for clients and
// performing the storage proofs on the received files.
type Host struct {
	// Module dependencies.
	cs     modules.ConsensusSet
	tpool  modules.TransactionPool
	wallet modules.Wallet

	// Consensus Tracking. 'actionItems' lists a bunch of contract obligations
	// that have 'todos' at a given height. The required action ranges from
	// needed to resubmit a revision to handling a storage proof or getting
	// pruned from the host.
	blockHeight  types.BlockHeight
	recentChange modules.ConsensusChangeID
	actionItems  map[types.BlockHeight]map[types.FileContractID]*contractObligation

	// Host Identity
	netAddress modules.NetAddress
	publicKey  types.SiaPublicKey
	secretKey  crypto.SecretKey

	// File Management.
	obligationsByID map[types.FileContractID]*contractObligation

	// Statistics
	anticipatedRevenue types.Currency
	fileCounter        int64
	lostRevenue        types.Currency
	revenue            types.Currency
	spaceRemaining     int64

	// RPC Tracking
	atomicErroredCalls   uint64
	atomicMalformedCalls uint64
	atomicDownloadCalls  uint64
	atomicRenewCalls     uint64
	atomicReviseCalls    uint64
	atomicSettingsCalls  uint64
	atomicUploadCalls    uint64

	// The resource lock is held by threaded functions for the duration of
	// their operation. Functions should grab the resource lock as a read lock
	// unless they are planning on manipulating the 'closed' variable.
	// Readlocks are used so that multiple functions can use resources
	// simultaneously, but the resources are not closed until all functions
	// accessing them have returned.
	closed       bool
	resourceLock sync.RWMutex

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

		actionItems: make(map[types.BlockHeight]map[types.FileContractID]*contractObligation),

		obligationsByID: make(map[types.FileContractID]*contractObligation),

		persistDir: persistDir,
	}

	// Load all of the saved host state into the host.
	err := h.initPersist()
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

// Close shuts down the host, preparing it for garbage collection.
func (h *Host) Close() error {
	// Unsubscribe the host from the consensus set. Call will not terminate
	// until the last consensus update has been sent to the host.
	// Unsubscription must happen before any resources are released or
	// terminated because the process consensus change function makes use of
	// those resources.
	h.cs.Unsubscribe(h)

	// Close the listener, which means incoming network connections will be
	// rejected. The listener should be closed before the host resources are
	// disabled, as incoming connections will want to use the hosts resources.
	err := h.listener.Close()
	if err != nil {
		return err
	}

	// Manage the port forwarding.
	err = h.clearPort(h.netAddress.Port())
	if err != nil {
		return err
	}

	// Grab the resource lock and indicate that the host is closing. Concurrent
	// functions hold the resource lock until they terminate, meaning that no
	// threaded function will be running by the time the resource lock is
	// acquired.
	h.resourceLock.Lock()
	h.closed = true
	h.resourceLock.Unlock()

	// Save the latest host state.
	h.mu.Lock()
	err = h.save()
	h.mu.Unlock()
	if err != nil {
		return err
	}

	// Close the logger.
	err = h.log.Close()
	if err != nil {
		return err
	}
	return nil
}

// addActionItem adds an action item at the given height for the given contract
// obligation.
func (h *Host) addActionItem(height types.BlockHeight, co *contractObligation) {
	_, exists := h.actionItems[height]
	if !exists {
		h.actionItems[height] = make(map[types.FileContractID]*contractObligation)
	}
	h.actionItems[height][co.ID] = co
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
	return h.anticipatedRevenue, h.revenue
}

// SetSettings updates the host's internal HostSettings object.
func (h *Host) SetSettings(settings modules.HostSettings) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.resourceLock.RLock()
	defer h.resourceLock.RUnlock()
	if h.closed {
		return errHostClosed
	}

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
