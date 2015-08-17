package renter

import (
	"errors"
	"os"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

var (
	ErrNilCS     = errors.New("cannot create renter with nil consensus set")
	ErrNilHostDB = errors.New("cannot create renter with nil hostdb")
	ErrNilWallet = errors.New("cannot create renter wil nil wallet")
	ErrNilTpool  = errors.New("cannot create renter wil nil transaction pool")
)

// A Renter is responsible for tracking all of the files that a user has
// uploaded to Sia, as well as the locations and health of these files.
type Renter struct {
	cs          modules.ConsensusSet
	hostDB      modules.HostDB
	wallet      modules.Wallet
	tpool       modules.TransactionPool
	blockHeight types.BlockHeight

	files         map[string]*file
	contracts     map[types.FileContractID]types.FileContract
	downloadQueue []*download
	saveDir       string

	mu *sync.RWMutex
}

// New returns an empty renter.
func New(cs modules.ConsensusSet, hdb modules.HostDB, wallet modules.Wallet, tpool modules.TransactionPool, saveDir string) (*Renter, error) {
	if cs == nil {
		return nil, ErrNilCS
	}
	if hdb == nil {
		return nil, ErrNilHostDB
	}
	if wallet == nil {
		return nil, ErrNilWallet
	}
	if tpool == nil {
		return nil, ErrNilTpool
	}

	r := &Renter{
		cs:     cs,
		hostDB: hdb,
		wallet: wallet,
		tpool:  tpool,

		files:     make(map[string]*file),
		contracts: make(map[types.FileContractID]types.FileContract),
		saveDir:   saveDir,

		mu: sync.New(modules.SafeMutexDelay, 1),
	}

	err := os.MkdirAll(saveDir, 0700)
	if err != nil {
		return nil, err
	}

	err = r.load()
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

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
	var totalPrice types.Currency
	redundancy := 6 // reasonable estimate until we come up with an alternative
	sampleSize := redundancy * 3 / 2
	hosts := r.hostDB.RandomHosts(sampleSize)
	for _, host := range hosts {
		totalPrice = totalPrice.Add(host.Price)
	}
	if len(hosts) == 0 {
		return
	}
	averagePrice := totalPrice.Mul(types.NewCurrency64(2)).Div(types.NewCurrency64(3))
	// HACK: 6000 is the duration (set by the API), and 1024^3 is a GB. Price
	// is reported as per GB, no timeframe is given.
	estimatedCost := averagePrice.Mul(types.NewCurrency64(6000)).Mul(types.NewCurrency64(1024 * 1024 * 1024))
	bufferedCost := estimatedCost.Mul(types.NewCurrency64(4)).Div(types.NewCurrency64(3))
	ri.Price = bufferedCost

	// Report the number of known hosts.
	ri.KnownHosts = len(r.hostDB.ActiveHosts())

	return
}

// enforce that Renter satisfies the modules.Renter interface
var _ modules.Renter = (*Renter)(nil)
