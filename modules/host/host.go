package host

import (
	"errors"
	"net"
	"os"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// StorageProofReorgDepth states how many blocks to wait before submitting
	// a storage proof. This reduces the chance of needing to resubmit because
	// of a reorg.
	StorageProofReorgDepth = 10
	maxContractLen         = 1 << 16 // The maximum allowed size of a file contract coming in over the wire. This does not include the file.
)

// A contractObligation tracks a file contract that the host is obligated to
// fulfill.
type contractObligation struct {
	ID           types.FileContractID
	FileContract types.FileContract
	Path         string // Where on disk the file is stored.
}

// A Host contains all the fields necessary for storing files for clients and
// performing the storage proofs on the received files.
type Host struct {
	cs          *consensus.State
	tpool       modules.TransactionPool
	wallet      modules.Wallet
	blockHeight types.BlockHeight

	myAddr         modules.NetAddress
	saveDir        string
	spaceRemaining int64
	fileCounter    int
	profit         types.Currency

	listener net.Listener

	obligationsByID     map[types.FileContractID]contractObligation
	obligationsByHeight map[types.BlockHeight][]contractObligation

	modules.HostSettings

	subscriptions []chan struct{}

	mu *sync.RWMutex
}

// New returns an initialized Host.
func New(cs *consensus.State, tpool modules.TransactionPool, wallet modules.Wallet, addr string, saveDir string) (h *Host, err error) {
	if cs == nil {
		err = errors.New("host cannot use a nil state")
		return
	}
	if tpool == nil {
		err = errors.New("host cannot use a nil tpool")
		return
	}
	if wallet == nil {
		err = errors.New("host cannot use a nil wallet")
		return
	}

	coinAddr, _, err := wallet.CoinAddress(false) // false indicates that the address should not be visible to the user.
	if err != nil {
		return
	}
	h = &Host{
		cs:     cs,
		tpool:  tpool,
		wallet: wallet,

		// default host settings
		HostSettings: modules.HostSettings{
			TotalStorage: 5e9,                       // 5 GB
			MaxFilesize:  1e9,                       // 1 GB
			MaxDuration:  144 * 30,                  // 30 days
			WindowSize:   288,                       // 48 hours
			Price:        types.NewCurrency64(1e15), // 1 siacoin / mb / week
			Collateral:   types.NewCurrency64(0),
			UnlockHash:   coinAddr,
		},

		saveDir: saveDir,

		obligationsByID:     make(map[types.FileContractID]contractObligation),
		obligationsByHeight: make(map[types.BlockHeight][]contractObligation),

		mu: sync.New(modules.SafeMutexDelay, 1),
	}
	h.spaceRemaining = h.TotalStorage

	// Create listener and set address.
	h.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return
	}
	_, port, _ := net.SplitHostPort(h.listener.Addr().String())
	h.myAddr = modules.NetAddress(net.JoinHostPort(modules.ExternalIP, port))

	err = os.MkdirAll(saveDir, 0700)
	if err != nil {
		return
	}
	h.load()

	// spawn listener
	go h.listen()

	h.cs.ConsensusSetSubscribe(h)

	return
}

// SetConfig updates the host's internal HostSettings object. To modify
// a specific field, use a combination of Info and SetConfig
func (h *Host) SetSettings(settings modules.HostSettings) {
	lockID := h.mu.Lock()
	defer h.mu.Unlock(lockID)
	h.spaceRemaining += settings.TotalStorage - h.TotalStorage
	h.HostSettings = settings
	h.save()
}

// Settings returns the settings of a host.
func (h *Host) Settings() modules.HostSettings {
	lockID := h.mu.RLock()
	defer h.mu.RUnlock(lockID)
	return h.HostSettings
}

func (h *Host) Address() modules.NetAddress {
	// no lock needed; h.myAddr is only set once (in New).
	return h.myAddr
}

func (h *Host) Info() modules.HostInfo {
	lockID := h.mu.RLock()
	defer h.mu.RUnlock(lockID)

	info := modules.HostInfo{
		HostSettings: h.HostSettings,

		StorageRemaining: h.spaceRemaining,
		NumContracts:     len(h.obligationsByID),
		Profit:           h.profit,
	}
	// sum up the current obligations to calculate PotentialProfit
	for _, obligation := range h.obligationsByID {
		fc := obligation.FileContract
		info.PotentialProfit = info.PotentialProfit.Add(fc.Payout.Sub(fc.Tax()))
	}

	return info
}
