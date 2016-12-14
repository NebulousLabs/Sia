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
	errNilCS    = errors.New("cannot create renter with nil consensus set")
	errNilTpool = errors.New("cannot create renter with nil transaction pool")
	errNilHdb   = errors.New("cannot create renter with nil hostdb")
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

	// Host returns the HostDBEntry for a given host.
	Host(modules.NetAddress) (modules.HostDBEntry, bool)
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

	// Metrics returns the financial metrics of the contractor.
	Metrics() (modules.RenterFinancialMetrics, []modules.RenterContractMetrics)

	// IsOffline reports whether the specified host is considered offline.
	IsOffline(modules.NetAddress) bool

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
	// modules
	cs modules.ConsensusSet

	// resources
	hostDB         hostDB
	hostContractor hostContractor
	log            *persist.Logger

	// variables
	files         map[string]*file
	tracking      map[string]trackedFile // map from nickname to metadata
	downloadQueue []*download
	uploading     bool
	downloading   bool

	// constants
	persistDir string

	mu *sync.RWMutex
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
	if hdb == nil {
		// Nil hdb currently allowed for testing purposes. :(
		// return nil, errNilHdb
	}

	r := &Renter{
		cs:             cs,
		hostDB:         hdb,
		hostContractor: hc,

		files:    make(map[string]*file),
		tracking: make(map[string]trackedFile),

		persistDir: persistDir,
		mu:         sync.New(modules.SafeMutexDelay, 1),
	}
	if err := r.initPersist(); err != nil {
		return nil, err
	}

	go r.threadedRepairLoop()

	return r, nil
}

// Close closes the Renter and its dependencies
func (r *Renter) Close() error {
	return r.hostDB.Close()
}

// hostdb passthroughs
func (r *Renter) ActiveHosts() []modules.HostDBEntry { return r.hostDB.ActiveHosts() }
func (r *Renter) AllHosts() []modules.HostDBEntry    { return r.hostDB.AllHosts() }

// contractor passthroughs
func (r *Renter) Contracts() []modules.RenterContract { return r.hostContractor.Contracts() }
func (r *Renter) Metrics() (modules.RenterFinancialMetrics, []modules.RenterContractMetrics) {
	return r.hostContractor.Metrics()
}
func (r *Renter) Settings() modules.RenterSettings {
	return modules.RenterSettings{
		Allowance: r.hostContractor.Allowance(),
	}
}
func (r *Renter) SetSettings(s modules.RenterSettings) error {
	return r.hostContractor.SetAllowance(s.Allowance)
}

// enforce that Renter satisfies the modules.Renter interface
var _ modules.Renter = (*Renter)(nil)
