package host

import (
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
)

// TODO: Changing the host path should automatically move all of the files
// over.

const (
	// StorageProofReorgDebth states how many blocks to wait before submitting
	// a storage proof. This reduces the chance of needing to resubmit because
	// of a reorg.
	StorageProofReorgDepth = 6
	maxContractLen         = 1 << 16 // The maximum allowed size of a file contract coming in over the wire.
)

type contractObligation struct {
	path string // Where on disk the file is stored.
}

type Host struct {
	state       *consensus.State
	tpool       modules.TransactionPool
	wallet      modules.Wallet
	latestBlock consensus.BlockID

	// our HostSettings, embedded for convenience
	modules.HostSettings

	hostDir        string
	spaceRemaining int64
	fileCounter    int

	contracts map[consensus.FileContractID]contractObligation

	mu sync.RWMutex
}

// New returns an initialized Host.
func New(state *consensus.State, wallet modules.Wallet) (h *Host, err error) {
	if wallet == nil {
		err = errors.New("host.New: cannot have nil wallet")
		return
	}
	if state == nil {
		err = errors.New("host.New: cannot have nil state")
		return
	}

	addr, _, err := wallet.CoinAddress()
	if err != nil {
		return
	}
	h = &Host{
		state:  state,
		wallet: wallet,

		// default host settings
		HostSettings: modules.HostSettings{
			MaxFilesize: 4 * 1000 * 1000,
			MaxDuration: 1008, // One week.
			MinWindow:   20,
			Price:       consensus.NewCurrency64(1),
			Collateral:  consensus.NewCurrency64(1),
			UnlockHash:  addr,
		},

		contracts: make(map[consensus.FileContractID]contractObligation),
	}

	return
}

// SetConfig updates the host's internal HostSettings object. To modify
// a specific field, use a combination of Info and SetConfig
func (h *Host) SetConfig(settings modules.HostSettings) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.HostSettings = settings
}

// Settings is an RPC used to request the settings of a host.
func (h *Host) Settings() (modules.HostSettings, error) {
	// TODO: return an error if we haven't announced yet
	return h.HostSettings, nil
}

func (h *Host) Info() modules.HostInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	info := modules.HostInfo{
		HostSettings: h.HostSettings,

		StorageRemaining: h.spaceRemaining,
		NumContracts:     len(h.contracts),
	}
	return info
}
