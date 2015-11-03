package renter

import (
	"crypto/rand"
	"errors"
	"log"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/hostdb"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

var (
	ErrNilCS     = errors.New("cannot create renter with nil consensus set")
	ErrNilHostDB = errors.New("cannot create renter with nil hostdb")
	ErrNilWallet = errors.New("cannot create renter with nil wallet")
	ErrNilTpool  = errors.New("cannot create renter with nil transaction pool")
)

// A hostDB is a database of hosts that the renter can use for figuring out who
// to upload to, and download from.
type hostDB interface {
	// ActiveHosts returns the list of hosts that are actively being selected
	// from.
	ActiveHosts() []modules.HostSettings

	// AllHosts returns the full list of hosts known to the hostdb.
	AllHosts() []modules.HostSettings

	// InsertHost adds a host to the database.
	InsertHost(modules.HostSettings) error

	// RandomHosts will pull up to 'num' random hosts from the hostdb. There
	// will be no repeats, but the length of the slice returned may be less
	// than 'num', and may even be 0. The hosts returned first have the higher
	// priority.
	RandomHosts(num int) []modules.HostSettings
}

// A Renter is responsible for tracking all of the files that a user has
// uploaded to Sia, as well as the locations and health of these files.
type Renter struct {
	// modules
	cs     modules.ConsensusSet
	wallet modules.Wallet
	tpool  modules.TransactionPool

	// resources
	hostDB hostDB
	log    *log.Logger

	// variables
	blockHeight   types.BlockHeight
	files         map[string]*file
	contracts     map[types.FileContractID]types.FileContract
	repairSet     map[string]string // map from nickname to filepath
	downloadQueue []*download
	cachedAddress types.UnlockHash // to prevent excessive address creation

	// constants
	persistDir string
	entropy    [32]byte // used to generate signing keys

	mu *sync.RWMutex
}

// New returns an empty renter.
func New(cs modules.ConsensusSet, wallet modules.Wallet, tpool modules.TransactionPool, persistDir string) (*Renter, error) {
	if cs == nil {
		return nil, ErrNilCS
	}
	if wallet == nil {
		return nil, ErrNilWallet
	}
	if tpool == nil {
		return nil, ErrNilTpool
	}

	hdb := hostdb.New()

	r := &Renter{
		cs:     cs,
		hostDB: hdb,
		wallet: wallet,
		tpool:  tpool,

		files:     make(map[string]*file),
		contracts: make(map[types.FileContractID]types.FileContract),
		repairSet: make(map[string]string),

		persistDir: persistDir,
		mu:         sync.New(modules.SafeMutexDelay, 1),
	}
	_, err := rand.Read(r.entropy[:])
	if err != nil {
		return nil, err
	}
	err = r.initPersist()
	if err != nil {
		return nil, err
	}

	cs.ConsensusSetSubscribe(r)

	go r.threadedRepairUploads()

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
	var totalPrice types.Currency
	sampleSize := defaultParityPieces + defaultDataPieces
	hosts := r.hostDB.RandomHosts(sampleSize)
	for _, host := range hosts {
		totalPrice = totalPrice.Add(host.Price)
	}
	if len(hosts) == 0 {
		return
	}
	averagePrice := totalPrice.Div(types.NewCurrency64(uint64(len(hosts))))
	estimatedCost := averagePrice.Mul(types.NewCurrency64(defaultDuration)).Mul(types.NewCurrency64(1e9)).Mul(types.NewCurrency64(defaultParityPieces + defaultDataPieces))
	// this also accounts for the buffering in the contract negotiation
	bufferedCost := estimatedCost.Mul(types.NewCurrency64(5)).Div(types.NewCurrency64(2))
	ri.Price = bufferedCost

	// Report the number of known hosts.
	ri.KnownHosts = len(r.hostDB.ActiveHosts())

	return
}

// enforce that Renter satisfies the modules.Renter interface
var _ modules.Renter = (*Renter)(nil)
