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
	defaultTotalStorage = 10e9         // 10 GB.
	defaultMaxDuration  = 144 * 30 * 6 // 6 months.
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
			return 144
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
	// RPC Metrics - atomic variables need to be placed at the top to preserve
	// compatibility with 32bit systems.
	atomicErroredCalls      uint64
	atomicUnrecognizedCalls uint64
	atomicDownloadCalls     uint64
	atomicRenewCalls        uint64
	atomicReviseCalls       uint64
	atomicSettingsCalls     uint64
	atomicUploadCalls       uint64

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

// DeleteContract deletes a file contract. The revenue and collateral on the
// file contract will be lost, and the data will be removed.
func (h *Host) DeleteContract(id types.FileContractID) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	obligation, exists := h.obligationsByID[id]
	if !exists {
		return errors.New("obligation not found")
	}
	h.removeObligation(obligation, obligationFailed)
	return nil
}
