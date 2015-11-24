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

	blockHeight         types.BlockHeight
	obligationsByID     map[types.FileContractID]*contractObligation
	obligationsByHeight map[types.BlockHeight][]*contractObligation
	spaceRemaining      int64
	fileCounter         int
	profit              types.Currency
	modules.HostSettings

	myAddr     modules.NetAddress
	persistDir string
	secretKey  crypto.SecretKey
	publicKey  types.SiaPublicKey

	listener net.Listener
	log      *log.Logger
	mu       sync.RWMutex
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
	sk, pk, err := crypto.StdKeyGen.Generate()
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
	h.myAddr = modules.NetAddress(h.listener.Addr().String())

	// Forward the hosting port, if possible.
	go h.forwardPort(h.myAddr.Port())

	// Learn our external IP.
	go h.learnHostname()

	// spawn listener
	go h.listen()

	h.cs.ConsensusSetSubscribe(h)

	return h, nil
}

// SetConfig updates the host's internal HostSettings object.
func (h *Host) SetSettings(settings modules.HostSettings) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.spaceRemaining += settings.TotalStorage - h.TotalStorage
	h.HostSettings = settings
	h.save()
}

// Settings returns the settings of a host.
func (h *Host) Settings() modules.HostSettings {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.HostSettings
}

func (h *Host) Address() modules.NetAddress {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.myAddr
}

func (h *Host) Info() modules.HostInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	h.HostSettings.IPAddress = h.myAddr // needs to be updated manually
	info := modules.HostInfo{
		HostSettings: h.HostSettings,

		StorageRemaining: h.spaceRemaining,
		NumContracts:     len(h.obligationsByID),
		Profit:           h.profit,
	}
	// sum up the current obligations to calculate PotentialProfit
	for _, obligation := range h.obligationsByID {
		fc := obligation.FileContract
		info.PotentialProfit = info.PotentialProfit.Add(types.PostTax(h.blockHeight, fc.Payout))
	}

	return info
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
	h.clearPort(h.myAddr.Port())
	// shut down the listener
	return h.listener.Close()
}
