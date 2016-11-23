package renter

import (
	"errors"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/contractor"
	"github.com/NebulousLabs/Sia/modules/renter/hostdb"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

var (
	errNilContractor = errors.New("cannot create renter with nil contractor")
	errNilCS         = errors.New("cannot create renter with nil consensus set")
	errNilTpool      = errors.New("cannot create renter with nil transaction pool")
	errNilHdb        = errors.New("cannot create renter with nil hostdb")
)

// A hostDB is a database of hosts that the renter can use for figuring out who
// to upload to, and download from.
type hostDB interface {
	// ActiveHosts returns the list of hosts that are actively being selected
	// from.
	ActiveHosts() []modules.HostDBEntry

	// AllHosts returns the full list of hosts known to the hostdb, sorted in
	// order of preference.
	AllHosts() []modules.HostDBEntry

	// AverageContractPrice returns the average contract price of a host.
	AverageContractPrice() types.Currency

	// Close closes the hostdb.
	Close() error

	// IsOffline reports whether a host is consider offline.
	IsOffline(modules.NetAddress) bool
}

// A hostContractor negotiates, revises, renews, and provides access to file
// contracts.
type hostContractor interface {
	// SetAllowance sets the amount of money the contractor is allowed to
	// spend on contracts over a given time period, divided among the number
	// of hosts specified. Note that contractor can start forming contracts as
	// soon as SetAllowance is called; that is, it may block.
	SetAllowance(modules.Allowance) error

	// Allowance returns the current allowance
	Allowance() modules.Allowance

	// Contract returns the latest contract formed with the specified host.
	Contract(modules.NetAddress) (modules.RenterContract, bool)

	// Contracts returns the contracts formed by the contractor.
	Contracts() []modules.RenterContract

	// Editor creates an Editor from the specified contract ID, allowing the
	// insertion, deletion, and modification of sectors.
	Editor(types.FileContractID) (contractor.Editor, error)

	// FinancialMetrics returns the financial metrics of the contractor.
	FinancialMetrics() modules.RenterFinancialMetrics

	// Downloader creates a Downloader from the specified contract ID,
	// allowing the retrieval of sectors.
	Downloader(types.FileContractID) (contractor.Downloader, error)
}

// A trackedFile contains metadata about files being tracked by the Renter.
// Tracked files are actively repaired by the Renter. By default, files
// uploaded by the user are tracked, and files that are added (via loading a
// .sia file) are not.
type trackedFile struct {
	// location of original file on disk
	RepairPath string
}

// A Renter is responsible for tracking all of the files that a user has
// uploaded to Sia, as well as the locations and health of these files.
type Renter struct {
	// File management.
	files    map[string]*file
	tracking map[string]trackedFile // map from nickname to metadata

	// Work management.
	workerPool    map[types.FileContractID]*worker
	downloadQueue []*download

	// Status Variables.
	uploading   bool
	downloading bool

	// Utilities.
	cs             modules.ConsensusSet
	hostContractor hostContractor
	hostDB         hostDB
	log            *persist.Logger
	persistDir     string
	mu             *sync.RWMutex
	tg             *sync.ThreadGroup
}

// New returns an initialized renter.
func New(cs modules.ConsensusSet, wallet modules.Wallet, tpool modules.TransactionPool, persistDir string) (*Renter, error) {
	hdb, err := hostdb.New(cs, persistDir)
	if err != nil {
		return nil, err
	}
	hc, err := contractor.New(cs, wallet, tpool, hdb, persistDir)
	if err != nil {
		return nil, err
	}

	return newRenter(cs, tpool, hdb, hc, persistDir)
}

// newRenter initializes a renter and returns it.
func newRenter(cs modules.ConsensusSet, tpool modules.TransactionPool, hdb hostDB, hc hostContractor, persistDir string) (*Renter, error) {
	if cs == nil {
		return nil, errNilCS
	}
	if tpool == nil {
		return nil, errNilTpool
	}
	if hc == nil {
		return nil, errNilContractor
	}
	if hdb == nil {
		// Nil hdb currently allowed for testing purposes. :(
		// return nil, errNilHdb
	}

	r := &Renter{
		files:    make(map[string]*file),
		tracking: make(map[string]trackedFile),

		workerPool: make(map[types.FileContractID]*worker),

		cs:             cs,
		hostDB:         hdb,
		hostContractor: hc,
		persistDir:     persistDir,
		mu:             sync.New(modules.SafeMutexDelay, 1),
		tg:             new(sync.ThreadGroup),
	}
	if err := r.initPersist(); err != nil {
		return nil, err
	}

	// Spin up the workers for the work pool.
	contracts := hc.Contracts()
	for _, c := range contracts {
		err := r.addWorker(c.ID)
		if err != nil {
			return nil, err
		}
	}

	go r.threadedRepairLoop()

	return r, nil
}

// Close closes the Renter and its dependencies
func (r *Renter) Close() error {
	r.tg.Stop()
	return r.hostDB.Close()
}

// SetSettings will update the settings for the renter.
func (r *Renter) SetSettings(s modules.RenterSettings) error {
	err := r.hostContractor.SetAllowance(s.Allowance)
	if err != nil {
		return err
	}

	id := r.mu.Lock()
	defer r.mu.Unlock(id)

	// Get the list of contracts in the contractor.
	newContracts := r.hostContractor.Contracts()

	// Get the list of contracts in the work pool.
	oldContracts := make([]types.FileContractID, 0, len(r.workerPool))
	for fcid := range r.workerPool {
		oldContracts = append(oldContracts, fcid)
	}

	// Find all elements in newContracts that are not in oldContracts and make
	// workers for them.
	for _, nc := range newContracts {
		found := false
		for _, ofcid := range oldContracts {
			if nc.ID == ofcid {
				found = true
				break
			}
		}
		if !found {
			err = r.addWorker(nc.ID)
			if err != nil {
				return err
			}
		}
	}

	// Find all elements in oldContracts that are not in newContracts and kill
	// their workers.
	for _, ocid := range oldContracts {
		found := false
		for _, nc := range newContracts {
			if nc.ID == ocid {
				found = true
				break
			}
		}
		if !found {
			err = r.retireWorker(ocid)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// hostdb passthroughs
func (r *Renter) ActiveHosts() []modules.HostDBEntry { return r.hostDB.ActiveHosts() }
func (r *Renter) AllHosts() []modules.HostDBEntry    { return r.hostDB.AllHosts() }

// contractor passthroughs
func (r *Renter) Contracts() []modules.RenterContract { return r.hostContractor.Contracts() }
func (r *Renter) FinancialMetrics() modules.RenterFinancialMetrics {
	return r.hostContractor.FinancialMetrics()
}
func (r *Renter) Settings() modules.RenterSettings {
	return modules.RenterSettings{
		Allowance: r.hostContractor.Allowance(),
	}
}

// enforce that Renter satisfies the modules.Renter interface
var _ modules.Renter = (*Renter)(nil)
