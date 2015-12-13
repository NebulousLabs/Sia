package renter

import (
	"log"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/hostdb"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

// A hostDB is a database of hosts that the renter can use for figuring out who
// to upload to, and download from.
type hostDB interface {
	// ActiveHosts returns the list of hosts that are actively being selected
	// from.
	ActiveHosts() []modules.HostSettings

	// AllHosts returns the full list of hosts known to the hostdb.
	AllHosts() []modules.HostSettings

	// AveragePrice returns the average price of a host.
	AveragePrice() types.Currency

	// NewPool returns a new HostPool, which can negotiate contracts with
	// hosts. The size and duration of these contracts are supplied as
	// arguments.
	NewPool(filesize uint64, duration types.BlockHeight) (hostdb.HostPool, error)

	// Renew renews a file contract, returning the new contract ID.
	Renew(id types.FileContractID, newHeight types.BlockHeight) (types.FileContractID, error)
}

// A trackedFile contains metadata about files being tracked by the Renter.
// Tracked files are actively repaired by the Renter.  By default, files
// uploaded by the user are tracked, and files that are added (via loading a
// .sia file) are not.
type trackedFile struct {
	// location of original file on disk
	RepairPath string
	// height at which file contracts should end. If EndHeight is 0, the file's
	// contracts will be renewed indefinitely.
	EndHeight types.BlockHeight
}

// A Renter is responsible for tracking all of the files that a user has
// uploaded to Sia, as well as the locations and health of these files.
type Renter struct {
	// modules
	cs     modules.ConsensusSet
	wallet modules.Wallet

	// resources
	hostDB hostDB
	log    *log.Logger

	// variables
	files         map[string]*file
	tracking      map[string]trackedFile // map from nickname to metadata
	downloadQueue []*download

	// constants
	persistDir string

	mu *sync.RWMutex
}

// New returns an empty renter.
func New(cs modules.ConsensusSet, wallet modules.Wallet, tpool modules.TransactionPool, persistDir string) (*Renter, error) {
	hdb, err := hostdb.New(cs, wallet, tpool, persistDir)
	if err != nil {
		return nil, err
	}

	r := &Renter{
		cs:     cs,
		wallet: wallet,
		hostDB: hdb,

		files:    make(map[string]*file),
		tracking: make(map[string]trackedFile),

		persistDir: persistDir,
		mu:         sync.New(modules.SafeMutexDelay, 1),
	}
	err = r.initPersist()
	if err != nil {
		return nil, err
	}

	go r.threadedRepairLoop()

	return r, nil
}

// Info returns generic information about the renter and the files that are
// being rented.
func (r *Renter) Info() (ri modules.RentInfo) {
	lockID := r.mu.RLock()
	// Include the list of files the renter knows about.
	for filename := range r.files {
		ri.Files = append(ri.Files, filename)
	}
	r.mu.RUnlock(lockID)

	// Calculate the average cost of a file.
	averagePrice := r.hostDB.AveragePrice()
	estimatedCost := averagePrice.Mul(types.NewCurrency64(defaultDuration)).Mul(types.NewCurrency64(1e9)).Mul(types.NewCurrency64(defaultParityPieces + defaultDataPieces))
	// this also accounts for the buffering in the contract negotiation
	bufferedCost := estimatedCost.Mul(types.NewCurrency64(5)).Div(types.NewCurrency64(2))
	ri.Price = bufferedCost

	// Report the number of known hosts.
	ri.KnownHosts = len(r.hostDB.ActiveHosts())

	return
}

// hostdb passthroughs
func (r *Renter) ActiveHosts() []modules.HostSettings { return r.hostDB.ActiveHosts() }
func (r *Renter) AllHosts() []modules.HostSettings    { return r.hostDB.AllHosts() }

// enforce that Renter satisfies the modules.Renter interface
var _ modules.Renter = (*Renter)(nil)
