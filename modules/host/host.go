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
	cs          *consensus.ConsensusSet
	hostdb      modules.HostDB
	tpool       modules.TransactionPool
	wallet      modules.Wallet
	blockHeight types.BlockHeight

	consensusHeight types.BlockHeight
	myAddr          modules.NetAddress
	saveDir         string
	spaceRemaining  int64
	fileCounter     int
	profit          types.Currency

	listener net.Listener

	obligationsByID     map[types.FileContractID]contractObligation
	obligationsByHeight map[types.BlockHeight][]contractObligation

	modules.HostSettings

	subscriptions []chan struct{}

	mu *sync.RWMutex
}

// New returns an initialized Host.
func New(cs *consensus.ConsensusSet, hdb modules.HostDB, tpool modules.TransactionPool, wallet modules.Wallet, addr string, saveDir string) (*Host, error) {
	if cs == nil {
		return nil, errors.New("host cannot use a nil state")
	}
	if hdb == nil {
		return nil, errors.New("host cannot use a nil hostdb")
	}
	if tpool == nil {
		return nil, errors.New("host cannot use a nil tpool")
	}
	if wallet == nil {
		return nil, errors.New("host cannot use a nil wallet")
	}

	coinAddr, _, err := wallet.CoinAddress(false) // false indicates that the address should not be visible to the user.
	if err != nil {
		return nil, err
	}
	h := &Host{
		cs:     cs,
		hostdb: hdb,
		tpool:  tpool,
		wallet: wallet,

		// default host settings
		HostSettings: modules.HostSettings{
			TotalStorage: 10e9,                        // 10 GB
			MaxFilesize:  1e9,                         // 1 GB
			MaxDuration:  144 * 60,                    // 60 days
			WindowSize:   288,                         // 48 hours
			Price:        types.NewCurrency64(100e12), // 0.1 siacoin / mb / week
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
		return nil, err
	}
	_, port, _ := net.SplitHostPort(h.listener.Addr().String())
	h.myAddr = modules.NetAddress(net.JoinHostPort(modules.ExternalIP, port))

	err = os.MkdirAll(saveDir, 0700)
	if err != nil {
		return nil, err
	}
	err = h.load()
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	// spawn listener
	go h.listen()

	h.cs.ConsensusSetSubscribe(h)

	return h, nil
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

	// Calculate estimated competition (reported in per GB per month). Price
	// calculated by taking the average of 8 ranomly selected weighted hosts.
	var averagePrice types.Currency
	// TODO: 8 is the sample size - to be made a constant.
	hosts := h.hostdb.RandomHosts(8)
	for _, host := range hosts {
		averagePrice = averagePrice.Add(host.Price)
	}
	if len(hosts) == 0 {
		return info
	}
	averagePrice = averagePrice.Div(types.NewCurrency64(uint64(len(hosts))))
	// HACK: 4320 is one month, and 1024^3 is a GB. Price is reported as per GB
	// per month.
	estimatedCost := averagePrice.Mul(types.NewCurrency64(4320)).Mul(types.NewCurrency64(1024 * 1024 * 1024))
	info.Competition = estimatedCost

	return info
}
