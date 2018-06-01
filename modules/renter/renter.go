// Package renter is responsible for uploading and downloading files on the sia
// network.
package renter

// NOTE: Some of the concurrency patterns in the renter are fairly complex. A
// lot of this has been cleaned up, though some shadows of previous demons still
// remain. Be careful when working with anything that involves concurrency.

// TODO: Allow the 'baseMemory' to be set by the user.

// TODO: The repair loop currently receives new upload jobs through a channel.
// The download loop has a better model, a heap that can be pushed to and popped
// from concurrently without needing complex channel communication. Migrating
// the renter to this model should clean up some of the places where uploading
// bottlenecks, and reduce the amount of channel-ninjitsu required to make the
// uploading function.

// TODO: Allow user to configure the packet size when ratelimiting the renter.
// Currently the default is set to 16kb. That's going to require updating the
// API and extending the settings object, and then tweaking the
// setBandwidthLimits function.

import (
	"errors"
	"reflect"
	"strings"
	"sync"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/contractor"
	"github.com/NebulousLabs/Sia/modules/renter/hostdb"
	"github.com/NebulousLabs/Sia/persist"
	siasync "github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/threadgroup"
)

var (
	errNilContractor = errors.New("cannot create renter with nil contractor")
	errNilCS         = errors.New("cannot create renter with nil consensus set")
	errNilGateway    = errors.New("cannot create hostdb with nil gateway")
	errNilHdb        = errors.New("cannot create renter with nil hostdb")
	errNilTpool      = errors.New("cannot create renter with nil transaction pool")
)

var (
	// priceEstimationScope is the number of hosts that get queried by the
	// renter when providing price estimates. Especially for the 'Standard'
	// variable, there should be congruence with the number of contracts being
	// used in the renter allowance.
	priceEstimationScope = build.Select(build.Var{
		Standard: int(50),
		Dev:      int(12),
		Testing:  int(4),
	}).(int)
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
	Host(types.SiaPublicKey) (modules.HostDBEntry, bool)

	// initialScanComplete returns a boolean indicating if the initial scan of the
	// hostdb is completed.
	InitialScanComplete() (bool, error)

	// RandomHosts returns a set of random hosts, weighted by their estimated
	// usefulness / attractiveness to the renter. RandomHosts will not return
	// any offline or inactive hosts.
	RandomHosts(int, []types.SiaPublicKey) ([]modules.HostDBEntry, error)

	// ScoreBreakdown returns a detailed explanation of the various properties
	// of the host.
	ScoreBreakdown(modules.HostDBEntry) modules.HostScoreBreakdown

	// EstimateHostScore returns the estimated score breakdown of a host with the
	// provided settings.
	EstimateHostScore(modules.HostDBEntry) modules.HostScoreBreakdown
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

	// Close closes the hostContractor.
	Close() error

	// Contracts returns the contracts formed by the contractor.
	Contracts() []modules.RenterContract

	// ContractByPublicKey returns the contract associated with the host key.
	ContractByPublicKey(types.SiaPublicKey) (modules.RenterContract, bool)

	// ContractUtility returns the utility field for a given contract, along
	// with a bool indicating if it exists.
	ContractUtility(types.SiaPublicKey) (modules.ContractUtility, bool)

	// CurrentPeriod returns the height at which the current allowance period
	// began.
	CurrentPeriod() types.BlockHeight

	// PeriodSpending returns the amount spent on contracts during the current
	// billing period.
	PeriodSpending() modules.ContractorSpending

	// Editor creates an Editor from the specified contract ID, allowing the
	// insertion, deletion, and modification of sectors.
	Editor(types.SiaPublicKey, <-chan struct{}) (contractor.Editor, error)

	// IsOffline reports whether the specified host is considered offline.
	IsOffline(types.SiaPublicKey) bool

	// Downloader creates a Downloader from the specified contract ID,
	// allowing the retrieval of sectors.
	Downloader(types.SiaPublicKey, <-chan struct{}) (contractor.Downloader, error)

	// ResolveIDToPubKey returns the public key of a host given a contract id.
	ResolveIDToPubKey(types.FileContractID) types.SiaPublicKey

	// RateLimits Gets the bandwidth limits for connections created by the
	// contractor and its submodules.
	RateLimits() (readBPS int64, writeBPS int64, packetSize uint64)

	// SetRateLimits sets the bandwidth limits for connections created by the
	// contractor and its submodules.
	SetRateLimits(int64, int64, uint64)
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
//
// TODO: Separate the workerPool to have its own mutex. The workerPool doesn't
// interfere with any of the other fields in the renter, should be fine for it
// to have a separate mutex, that way operations on the worker pool don't block
// operations on other parts of the struct. If we're going to do it that way,
// might make sense to split the worker pool off into it's own struct entirely
// the same way that we split of the memoryManager entirely.
type Renter struct {
	// File management.
	//
	// tracking contains a list of files that the user intends to maintain. By
	// default, files loaded through sharing are not maintained by the user.
	files map[string]*file

	// Download management. The heap has a separate mutex because it is always
	// accessed in isolation.
	downloadHeapMu sync.Mutex         // Used to protect the downloadHeap.
	downloadHeap   *downloadChunkHeap // A heap of priority-sorted chunks to download.
	newDownloads   chan struct{}      // Used to notify download loop that new downloads are available.

	// Download history. The history list has its own mutex because it is always
	// accessed in isolation.
	//
	// TODO: Currently the download history doesn't include repair-initiated
	// downloads, and instead only contains user-initiated downlods.
	downloadHistory   []*download
	downloadHistoryMu sync.Mutex

	// Upload management.
	uploadHeap uploadHeap

	// List of workers that can be used for uploading and/or downloading.
	memoryManager *memoryManager
	workerPool    map[types.FileContractID]*worker

	// Cache the last price estimation result.
	lastEstimation modules.RenterPriceEstimation

	// Utilities.
	staticStreamCache *streamCache
	cs                modules.ConsensusSet
	deps              modules.Dependencies
	g                 modules.Gateway
	hostContractor    hostContractor
	hostDB            hostDB
	log               *persist.Logger
	persist           persistence
	persistDir        string
	mu                *siasync.RWMutex
	tg                threadgroup.ThreadGroup
	tpool             modules.TransactionPool
}

// Close closes the Renter and its dependencies
func (r *Renter) Close() error {
	r.tg.Stop()
	r.hostDB.Close()
	return r.hostContractor.Close()
}

// PriceEstimation estimates the cost in siacoins of performing various storage
// and data operations.
//
// TODO: Make this function line up with the actual settings in the renter.
// Perhaps even make it so it uses the renter's actual contracts if it has any.
func (r *Renter) PriceEstimation() modules.RenterPriceEstimation {
	id := r.mu.RLock()
	lastEstimation := r.lastEstimation
	r.mu.RUnlock(id)
	if !reflect.DeepEqual(lastEstimation, modules.RenterPriceEstimation{}) {
		return lastEstimation
	}

	// Grab hosts to perform the estimation.
	hosts, err := r.hostDB.RandomHosts(priceEstimationScope, nil)
	if err != nil {
		return modules.RenterPriceEstimation{}
	}

	// Check if there are zero hosts, which means no estimation can be made.
	if len(hosts) == 0 {
		return modules.RenterPriceEstimation{}
	}

	// Add up the costs for each host.
	var totalContractCost types.Currency
	var totalDownloadCost types.Currency
	var totalStorageCost types.Currency
	var totalUploadCost types.Currency
	for _, host := range hosts {
		totalContractCost = totalContractCost.Add(host.ContractPrice)
		totalDownloadCost = totalDownloadCost.Add(host.DownloadBandwidthPrice)
		totalStorageCost = totalStorageCost.Add(host.StoragePrice)
		totalUploadCost = totalUploadCost.Add(host.UploadBandwidthPrice)
	}

	// Convert values to being human-scale.
	totalDownloadCost = totalDownloadCost.Mul(modules.BytesPerTerabyte)
	totalStorageCost = totalStorageCost.Mul(modules.BlockBytesPerMonthTerabyte)
	totalUploadCost = totalUploadCost.Mul(modules.BytesPerTerabyte)

	// Factor in redundancy.
	totalStorageCost = totalStorageCost.Mul64(3) // TODO: follow file settings?
	totalUploadCost = totalUploadCost.Mul64(3)   // TODO: follow file settings?

	// Perform averages.
	totalContractCost = totalContractCost.Div64(uint64(len(hosts)))
	totalDownloadCost = totalDownloadCost.Div64(uint64(len(hosts)))
	totalStorageCost = totalStorageCost.Div64(uint64(len(hosts)))
	totalUploadCost = totalUploadCost.Div64(uint64(len(hosts)))

	// Take the average of the host set to estimate the overall cost of the
	// contract forming.
	totalContractCost = totalContractCost.Mul64(uint64(priceEstimationScope))

	// Add the cost of paying the transaction fees for the first contract.
	_, feePerByte := r.tpool.FeeEstimation()
	totalContractCost = totalContractCost.Add(feePerByte.Mul64(1000).Mul64(uint64(priceEstimationScope)))

	est := modules.RenterPriceEstimation{
		FormContracts:        totalContractCost,
		DownloadTerabyte:     totalDownloadCost,
		StorageTerabyteMonth: totalStorageCost,
		UploadTerabyte:       totalUploadCost,
	}

	id = r.mu.Lock()
	r.lastEstimation = est
	r.mu.Unlock(id)

	return est
}

// setBandwidthLimits will change the bandwidth limits of the renter based on
// the persist values for the bandwidth.
func (r *Renter) setBandwidthLimits(downloadSpeed int64, uploadSpeed int64) error {
	// Input validation.
	if downloadSpeed < 0 || uploadSpeed < 0 {
		return errors.New("download/upload rate limit can't be below 0")
	}

	// Check for sentinal "no limits" value.
	if downloadSpeed == 0 && uploadSpeed == 0 {
		r.hostContractor.SetRateLimits(0, 0, 0)
	} else {
		// Set the rate limits according to the provided values.
		r.hostContractor.SetRateLimits(downloadSpeed, uploadSpeed, 4*4096)
	}
	return nil
}

// SetSettings will update the settings for the renter.
//
// NOTE: This function can't be atomic. Typically we try to have user requests
// be atomic, so that either everything changes or nothing changes, but since
// these changes happen progressively, it's possible for some of the settings
// (like the allowance) to succeed, but then if the bandwidth limits for example
// are bad, then the allowance will update but the bandwidth will not update.
func (r *Renter) SetSettings(s modules.RenterSettings) error {
	// Early input valudation.
	if s.MaxDownloadSpeed < 0 || s.MaxUploadSpeed < 0 {
		return errors.New("bandwidth limits cannot be negative")
	}
	if s.StreamCacheSize <= 0 {
		return errors.New("stream cache size needs to be 1 or larger")
	}

	// Set allowance.
	err := r.hostContractor.SetAllowance(s.Allowance)
	if err != nil {
		return err
	}

	// Set the bandwidth limits.
	err = r.setBandwidthLimits(s.MaxDownloadSpeed, s.MaxUploadSpeed)
	if err != nil {
		return err
	}
	r.persist.MaxDownloadSpeed = s.MaxDownloadSpeed
	r.persist.MaxUploadSpeed = s.MaxUploadSpeed

	// Set StreamingCacheSize
	err = r.staticStreamCache.SetStreamingCacheSize(s.StreamCacheSize)
	if err != nil {
		return err
	}
	r.persist.StreamCacheSize = s.StreamCacheSize

	// Save the changes.
	err = r.saveSync()
	if err != nil {
		return err
	}

	// Update the worker pool so that the changes are immediately apparent to
	// users.
	r.managedUpdateWorkerPool()
	return nil
}

// ActiveHosts returns an array of hostDB's active hosts
func (r *Renter) ActiveHosts() []modules.HostDBEntry { return r.hostDB.ActiveHosts() }

// AllHosts returns an array of all hosts
func (r *Renter) AllHosts() []modules.HostDBEntry { return r.hostDB.AllHosts() }

// Host returns the host associated with the given public key
func (r *Renter) Host(spk types.SiaPublicKey) (modules.HostDBEntry, bool) { return r.hostDB.Host(spk) }

// InitialScanComplete returns a boolean indicating if the initial scan of the
// hostdb is completed.
func (r *Renter) InitialScanComplete() (bool, error) { return r.hostDB.InitialScanComplete() }

// ScoreBreakdown returns the score breakdown
func (r *Renter) ScoreBreakdown(e modules.HostDBEntry) modules.HostScoreBreakdown {
	return r.hostDB.ScoreBreakdown(e)
}

// EstimateHostScore returns the estimated host score
func (r *Renter) EstimateHostScore(e modules.HostDBEntry) modules.HostScoreBreakdown {
	return r.hostDB.EstimateHostScore(e)
}

// Contracts returns an array of host contractor's contracts
func (r *Renter) Contracts() []modules.RenterContract { return r.hostContractor.Contracts() }

// CurrentPeriod returns the host contractor's current period
func (r *Renter) CurrentPeriod() types.BlockHeight { return r.hostContractor.CurrentPeriod() }

// ContractUtility returns the utility field for a given contract, along
// with a bool indicating if it exists.
func (r *Renter) ContractUtility(pk types.SiaPublicKey) (modules.ContractUtility, bool) {
	return r.hostContractor.ContractUtility(pk)
}

// PeriodSpending returns the host contractor's period spending
func (r *Renter) PeriodSpending() modules.ContractorSpending { return r.hostContractor.PeriodSpending() }

// Settings returns the host contractor's allowance
func (r *Renter) Settings() modules.RenterSettings {
	download, upload, _ := r.hostContractor.RateLimits()
	return modules.RenterSettings{
		Allowance:        r.hostContractor.Allowance(),
		MaxDownloadSpeed: download,
		MaxUploadSpeed:   upload,
		StreamCacheSize:  r.staticStreamCache.cacheSize,
	}
}

// ProcessConsensusChange returns the process consensus change
func (r *Renter) ProcessConsensusChange(cc modules.ConsensusChange) {
	id := r.mu.Lock()
	r.lastEstimation = modules.RenterPriceEstimation{}
	r.mu.Unlock(id)
}

// validateSiapath checks that a Siapath is a legal filename.
// ../ is disallowed to prevent directory traversal, and paths must not begin
// with / or be empty.
func validateSiapath(siapath string) error {
	if siapath == "" {
		return ErrEmptyFilename
	}
	if siapath == ".." {
		return errors.New("siapath cannot be '..'")
	}
	if siapath == "." {
		return errors.New("siapath cannot be '.'")
	}
	// check prefix
	if strings.HasPrefix(siapath, "/") {
		return errors.New("siapath cannot begin with /")
	}
	if strings.HasPrefix(siapath, "../") {
		return errors.New("siapath cannot begin with ../")
	}
	if strings.HasPrefix(siapath, "./") {
		return errors.New("siapath connot begin with ./")
	}
	for _, pathElem := range strings.Split(siapath, "/") {
		if pathElem == "." || pathElem == ".." {
			return errors.New("siapath cannot contain . or .. elements")
		}
	}
	return nil
}

// Enforce that Renter satisfies the modules.Renter interface.
var _ modules.Renter = (*Renter)(nil)

// NewCustomRenter initializes a renter and returns it.
func NewCustomRenter(g modules.Gateway, cs modules.ConsensusSet, tpool modules.TransactionPool, hdb hostDB, hc hostContractor, persistDir string, deps modules.Dependencies) (*Renter, error) {
	if g == nil {
		return nil, errNilGateway
	}
	if cs == nil {
		return nil, errNilCS
	}
	if tpool == nil {
		return nil, errNilTpool
	}
	if hc == nil {
		return nil, errNilContractor
	}
	if hdb == nil && build.Release != "testing" {
		return nil, errNilHdb
	}

	r := &Renter{
		files: make(map[string]*file),

		// Making newDownloads a buffered channel means that most of the time, a
		// new download will trigger an unnecessary extra iteration of the
		// download heap loop, searching for a chunk that's not there. This is
		// preferable to the alternative, where in rare cases the download heap
		// will miss work altogether.
		newDownloads: make(chan struct{}, 1),
		downloadHeap: new(downloadChunkHeap),

		uploadHeap: uploadHeap{
			activeChunks: make(map[uploadChunkID]struct{}),
			newUploads:   make(chan struct{}, 1),
		},

		workerPool: make(map[types.FileContractID]*worker),

		cs:             cs,
		deps:           deps,
		g:              g,
		hostDB:         hdb,
		hostContractor: hc,
		persistDir:     persistDir,
		mu:             siasync.New(modules.SafeMutexDelay, 1),
		tpool:          tpool,
	}
	r.memoryManager = newMemoryManager(defaultMemory, r.tg.StopChan())

	// Load all saved data.
	if err := r.initPersist(); err != nil {
		return nil, err
	}

	// Set the bandwidth limits, sincce the contractor doesn't persist them.
	//
	// TODO: Reconsider the way that the bandwidth limits are allocated to the
	// renter module, becaause really it seems they only impact the contractor.
	// The renter itself doesn't actually do any uploading or downloading.
	err := r.setBandwidthLimits(r.persist.MaxDownloadSpeed, r.persist.MaxUploadSpeed)
	if err != nil {
		return nil, err
	}

	// Initialize the streaming cache.
	r.staticStreamCache = newStreamCache(r.persist.StreamCacheSize)

	// Subscribe to the consensus set.
	err = cs.ConsensusSetSubscribe(r, modules.ConsensusChangeRecent, r.tg.StopChan())
	if err != nil {
		return nil, err
	}

	// Spin up the workers for the work pool.
	r.managedUpdateWorkerPool()
	go r.threadedDownloadLoop()
	go r.threadedUploadLoop()

	// Kill workers on shutdown.
	r.tg.OnStop(func() error {
		id := r.mu.RLock()
		for _, worker := range r.workerPool {
			close(worker.killChan)
		}
		r.mu.RUnlock(id)
		return nil
	})

	return r, nil
}

// New returns an initialized renter.
func New(g modules.Gateway, cs modules.ConsensusSet, wallet modules.Wallet, tpool modules.TransactionPool, persistDir string) (*Renter, error) {
	hdb, err := hostdb.New(g, cs, persistDir)
	if err != nil {
		return nil, err
	}
	hc, err := contractor.New(cs, wallet, tpool, hdb, persistDir)
	if err != nil {
		return nil, err
	}

	return NewCustomRenter(g, cs, tpool, hdb, hc, persistDir, modules.ProdDependencies)
}
