package renter

import (
	"errors"
	"os"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

var (
	ErrNilCS     = errors.New("cannot create renter with nil consensus set")
	ErrNilHostDB = errors.New("cannot create renter with nil hostdb")
	ErrNilWallet = errors.New("cannot create renter wil nil wlalet")
)

// A Renter is responsible for tracking all of the files that a user has
// uploaded to Sia, as well as the locations and health of these files.
type Renter struct {
	cs          *consensus.State
	hostDB      modules.HostDB
	wallet      modules.Wallet
	blockHeight types.BlockHeight

	files         map[string]*file
	downloadQueue []*Download
	saveDir       string

	subscriptions []chan struct{}

	mu *sync.RWMutex
}

// New returns an empty renter.
func New(cs *consensus.State, hdb modules.HostDB, wallet modules.Wallet, saveDir string) (*Renter, error) {
	if cs == nil {
		return nil, ErrNilCS
	}
	if hdb == nil {
		return nil, ErrNilHostDB
	}
	if wallet == nil {
		return nil, ErrNilWallet
	}

	r := &Renter{
		cs:     cs,
		hostDB: hdb,
		wallet: wallet,

		files:   make(map[string]*file),
		saveDir: saveDir,

		mu: sync.New(modules.SafeMutexDelay, 1),
	}

	err := os.MkdirAll(saveDir, 0700)
	if err != nil {
		return nil, err
	}

	r.load()

	// TODO: I'm worried about balances here. Because of the way that the
	// re-try algorithm works, it won't be a problem, but without that we would
	// need to make sure that scanAllFiles() didn't get called until the entire
	// balance had loaded, which would require loading the entire blockchain.
	// This also won't be a problem once we're also saving the addresses.
	//
	// TODO: bring back this functionality when we have resumable uploads.
	//r.scanAllFiles()

	r.cs.ConsensusSetSubscribe(r)

	return r, nil
}

// Info returns generic information about the renter and the files that are
// being rented.
func (r *Renter) Info() (ri modules.RentInfo) {
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

	// Include the list of files the renter knows about.
	for filename := range r.files {
		ri.Files = append(ri.Files, filename)
	}

	// Calculate the average cost of a file.
	var averagePrice types.Currency
	sampleSize := redundancy * 2
	hosts := r.hostDB.RandomHosts(sampleSize)
	for _, host := range hosts {
		averagePrice = averagePrice.Add(host.Price)
	}
	averagePrice = averagePrice.Div(types.NewCurrency64(uint64(len(hosts))))
	// HACK: 6000 is the duration (set by the API), and 1024^3 is a GB. Price
	// is reported as per GB, no timeframe is given.
	estimatedCost := averagePrice.Mul(types.NewCurrency64(6000)).Mul(types.NewCurrency64(1024 * 1024 * 1024))
	bufferedCost := estimatedCost.Mul(types.NewCurrency64(2))
	ri.Price = bufferedCost

	return
}
