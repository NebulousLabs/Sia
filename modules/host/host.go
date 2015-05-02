package host

import (
	"errors"
	"io"
	"net"
	"net/http"
	"os"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// StorageProofReorgDepth states how many blocks to wait before submitting
	// a storage proof. This reduces the chance of needing to resubmit because
	// of a reorg.
	StorageProofReorgDepth = 20
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

	coinAddr, _, err := wallet.CoinAddress()
	if err != nil {
		return
	}
	h = &Host{
		cs:     cs,
		tpool:  tpool,
		wallet: wallet,

		// default host settings
		HostSettings: modules.HostSettings{
			TotalStorage: 2e9,                      // 2 GB
			MaxFilesize:  300e6,                    // 300 MB
			MaxDuration:  5e3,                      // Just over a month.
			WindowSize:   288,                      // 48 hours.
			Price:        types.NewCurrency64(1e9), // 10^9
			Collateral:   types.NewCurrency64(0),
			UnlockHash:   coinAddr,
		},

		saveDir:        saveDir,
		spaceRemaining: 2e9,

		obligationsByID:     make(map[types.FileContractID]contractObligation),
		obligationsByHeight: make(map[types.BlockHeight][]contractObligation),

		mu: sync.New(modules.SafeMutexDelay, 1),
	}

	h.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return
	}
	h.myAddr = modules.NetAddress(h.listener.Addr().String())

	// discover external IP (during testing, use the loopback address)
	var hostname string
	if build.Release == "testing" {
		hostname = "::1"
	} else {
		hostname, err = getExternalIP()
		if err != nil {
			return nil, err
		}
	}
	h.myAddr = modules.NetAddress(net.JoinHostPort(hostname, h.myAddr.Port()))

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
	}
	return info
}

// getExternalIP learns the server's hostname from a centralized service,
// myexternalip.com.
func getExternalIP() (string, error) {
	resp, err := http.Get("http://myexternalip.com/raw")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	buf := make([]byte, 64)
	n, err := resp.Body.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}
	hostname := string(buf[:n-1]) // trim newline
	return hostname, nil
}
