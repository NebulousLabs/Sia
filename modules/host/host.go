package host

import (
	"errors"
	"log"
	"net"
	"sync"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
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
	defaultPrice = types.SiacoinPrecision.Div(types.NewCurrency64(4320e9 / 200)) // 200 SC / GB / Month
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

	// File management.
	fileCounter         int
	obligationsByID     map[types.FileContractID]*contractObligation
	obligationsByHeight map[types.BlockHeight][]*contractObligation
	profit              types.Currency
	spaceRemaining      int64

	// Persistent settings.
	blockHeight types.BlockHeight
	netAddr     modules.NetAddress
	secretKey   crypto.SecretKey
	publicKey   types.SiaPublicKey
	modules.HostSettings

	// Utilities.
	listener   net.Listener
	log        *log.Logger
	mu         sync.RWMutex
	persistDir string
}

// New returns an initialized Host.
func New(cs modules.ConsensusSet, tpool modules.TransactionPool, wallet modules.Wallet, addr string, persistDir string) (*Host, error) {
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

		// default host settings
		HostSettings: modules.HostSettings{
			TotalStorage: 10e9,         // 10 GB
			MaxFilesize:  100e9,        // 100 GB
			MaxDuration:  144 * 60,     // 60 days
			WindowSize:   288,          // 48 hours
			Price:        defaultPrice, // 200 SC / GB / Month
			Collateral:   types.NewCurrency64(0),
		},

		persistDir: persistDir,

		obligationsByID:     make(map[types.FileContractID]*contractObligation),
		obligationsByHeight: make(map[types.BlockHeight][]*contractObligation),
	}
	h.spaceRemaining = h.TotalStorage

	// Generate signing key, for revising contracts.
	sk, pk, err := crypto.GenerateKeyPair()
	if err != nil {
		return nil, err
	}
	h.secretKey = sk
	h.publicKey = types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       pk[:],
	}

	// Load the old host data and initialize the logger.
	err = h.initPersist()
	if err != nil {
		return nil, err
	}

	// Create listener and set address.
	h.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	h.netAddr = modules.NetAddress(h.listener.Addr().String())

	// Forward the hosting port, if possible.
	go h.forwardPort(h.netAddr.Port())

	// Learn our external IP.
	go h.learnHostname()

	// spawn listener
	go h.listen()

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
	return h.netAddr
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
	h.spaceRemaining += settings.TotalStorage - h.TotalStorage
	h.HostSettings = settings
	h.save()
}

// Settings returns the settings of a host.
func (h *Host) Settings() modules.HostSettings {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.HostSettings
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
	h.clearPort(h.netAddr.Port())
	// shut down the listener
	return h.listener.Close()
}
