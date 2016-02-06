// Package host is an implementation of the host module, and is responsible for
// storing data and advertising available storage to the network.
package host

import (
	"errors"
	"net"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

const (
	defaultTotalStorage = 10e9         // 10 GB.
	defaultMaxDuration  = 144 * 30 * 6 // 6 months.

	// Names of the various persistent files in the host.
	dbFilename   = modules.HostDir + ".db"
	logFile      = modules.HostDir + ".log"
	settingsFile = modules.HostDir + ".json"
)

var (
	// dbMetadata is a header that gets put into the database to identify a
	// version and indicate that the database holds host information.
	dbMetadata = persist.Metadata{
		Header:  "Sia Host DB",
		Version: "0.5.2",
	}

	// persistMetadata is the header that gets written to the persist file, and is
	// used to recognize other persist files.
	persistMetadata = persist.Metadata{
		Header:  "Sia Host",
		Version: "0.5",
	}

	// defaultPrice defines the starting price for hosts selling storage. We
	// try to match a number that is both reasonably profitable and reasonably
	// competitive.
	defaultPrice = modules.StoragePriceToConsensus(100e3) // 100 SC / GB / Month

	// defaultCollateral defines the amount of money that the host puts up as
	// collateral per-byte by default. Set to zero currently because neither of
	// the negotiation protocols have logic to deal with non-zero collateral.
	defaultCollateral = types.NewCurrency64(0)

	// defaultWindowSize is the size of the proof of storage window requested
	// by the host. The host will not delete any obligations until the window
	// has closed and buried under several confirmations.
	defaultWindowSize = func() types.BlockHeight {
		if build.Release == "testing" {
			return 5
		}
		if build.Release == "standard" {
			return 144
		}
		if build.Release == "dev" {
			return 36
		}
		panic("unrecognized release constant in host")
	}()

	// errChangedUnlockHash is returned by SetSettings if the unlock hash has
	// changed, an illegal operation.
	errChangedUnlockHash = errors.New("cannot change the unlock hash in SetSettings")

	// errHostClosed gets returned when a call is rejected due to the host
	// having been closed.
	errHostClosed = errors.New("call is disabled because the host is closed")

	// Nil dependency errors.
	errNilCS     = errors.New("host cannot use a nil state")
	errNilTpool  = errors.New("host cannot use a nil transaction pool")
	errNilWallet = errors.New("host cannot use a nil wallet")
)

// All of the following variables define the names of buckets used by the host
// in the database.
var (
	// BucketActionItems maps a blockchain height to a list of storage
	// obligations that need to be managed in some way at that height. The
	// height is stored as a big endian uint64, which means that bolt will
	// store the heights sorted in numerical order. The action item itself is
	// an array of file contract ids. The host is able to contextually figure
	// out what the necessary actions for that item are based on the file
	// contract id and the associated storage obligation that can be retreived
	// using the id.
	BucketActionItems = []byte("BucketActionItems")

	// BucketSectorUsage maps sector IDs to the number of times they are used
	// in file contracts. If all data is correctly encrypted using a unique
	// seed, each sector will be in use exactly one time. The host however
	// cannot control this, and a user may upload unencrypted data or
	// intentionally upload colliding sectors as a means of attack. The host
	// can only delete a sector when it is in use zero times. The number of
	// times a sector is in use is encoded as a big endian uint64.
	BucketSectorUsage = []byte("BucketSectorUsage")

	// BucketStorageObligations contains a set of serialized
	// 'storageObligations' sorted by their file contract id.
	BucketStorageObligations = []byte("BucketStorageObligations")
)

// A Host contains all the fields necessary for storing files for clients and
// performing the storage proofs on the received files.
type Host struct {
	// RPC Metrics - atomic variables need to be placed at the top to preserve
	// compatibility with 32bit systems.
	atomicErroredCalls      uint64
	atomicUnrecognizedCalls uint64
	atomicDownloadCalls     uint64
	atomicRenewCalls        uint64
	atomicReviseCalls       uint64
	atomicSettingsCalls     uint64
	atomicUploadCalls       uint64

	// Module dependencies.
	cs     modules.ConsensusSet
	tpool  modules.TransactionPool
	wallet modules.Wallet

	// Consensus Tracking.
	blockHeight  types.BlockHeight
	recentChange modules.ConsensusChangeID

	// Host Identity
	netAddress modules.NetAddress
	publicKey  types.SiaPublicKey
	secretKey  crypto.SecretKey

	// File Management - 'actionItems' lists a bunch of contract obligations
	// that have 'todos' at a given height. The required action ranges from
	// needed to resubmit a revision to handling a storage proof or getting
	// pruned from the host.
	//
	// DEPRECATED v0.5.2
	obligationsByID map[types.FileContractID]*contractObligation
	actionItems     map[types.BlockHeight]map[types.FileContractID]*contractObligation

	// Storage Obligation Management - different from file management in that
	// the storage obligation management is the new way of handling storage
	// obligations. Is a replacement for the contract obligation logic, but the
	// old logic is being kept for compatibility purposes.
	lockedStorageObligations map[types.FileContractID]struct{} // Which storage obligations are currently being modified.

	// Statistics
	anticipatedRevenue types.Currency
	fileCounter        int64
	lostRevenue        types.Currency
	revenue            types.Currency
	spaceRemaining     int64

	// Utilities.
	db         *persist.BoltDatabase
	listener   net.Listener
	log        *persist.Logger
	mu         sync.RWMutex
	persistDir string
	settings   modules.HostSettings

	// The resource lock is held by threaded functions for the duration of
	// their operation. Functions should grab the resource lock as a read lock
	// unless they are planning on manipulating the 'closed' variable.
	// Readlocks are used so that multiple functions can use resources
	// simultaneously, but the resources are not closed until all functions
	// accessing them have returned.
	closed       bool
	resourceLock sync.RWMutex

	// Dependencies
	persister
}

// initDB will check that the database has been initialized and if not, will
// initialize the database.
func (h *Host) initDB() error {
	return h.db.Update(func(tx *bolt.Tx) error {
		// Return nil if the database is already initialized. The database can
		// be safely assumed to be initialized if the storage obligation bucket
		// exists.
		bso := tx.Bucket(BucketStorageObligations)
		if bso != nil {
			return nil
		}

		// The storage obligation bucket does not exist, which means the
		// database needs to be initialized. Create the database buckets.
		buckets := [][]byte{
			BucketActionItems,
			BucketSectorUsage,
			BucketStorageObligations,
		}
		for _, bucket := range buckets {
			_, err := tx.CreateBucket(bucket)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// newHost returns an initialized Host, taking a series of dependencies in as
// arguments. By making the dependencies arguments of the 'new' call, the host
// can be mocked such that the dependencies can return unexpected errors or
// unique behaviors during testing, enabling easier testing of the failure
// modes of the Host.
func newHost(persister persister, cs modules.ConsensusSet, tpool modules.TransactionPool, wallet modules.Wallet, address string, persistDir string) (*Host, error) {
	// Check that all the dependencies were provided.
	if cs == nil {
		return nil, errNilCS
	}
	if tpool == nil {
		return nil, errNilTpool
	}
	if wallet == nil {
		return nil, errNilWallet
	}

	// Create the host object.
	h := &Host{
		cs:     cs,
		tpool:  tpool,
		wallet: wallet,

		actionItems: make(map[types.BlockHeight]map[types.FileContractID]*contractObligation),

		obligationsByID: make(map[types.FileContractID]*contractObligation),

		lockedStorageObligations: make(map[types.FileContractID]struct{}),

		persistDir: persistDir,

		persister: persister,
	}

	// Create the perist directory if it does not yet exist.
	err := persister.MkdirAll(h.persistDir, 0700)
	if err != nil {
		return nil, err
	}

	// Initialize the logger. Logging should be initialized ASAP, because the
	// rest of the initialization makes use of the logger.
	h.log, err = persist.NewLogger(filepath.Join(h.persistDir, logFile))
	if err != nil {
		return nil, err
	}

	// Open the database containing the host's storage obligation metadata.
	h.db, err = persist.OpenDatabase(dbMetadata, filepath.Join(h.persistDir, dbFilename))
	if err != nil {
		// An error will be returned if the database has the wrong version, but
		// as of writing there was only one version of the database and all
		// other databases would be incompatible.
		_ = h.log.Close()
		return nil, err
	}
	// After opening the database, it must be initalized. Most commonly,
	// nothing happens. But for new databases, a set of buckets must be
	// created. Intialization is also a good time to run sanity checks.
	err = h.initDB()
	if err != nil {
		_ = h.log.Close()
		_ = h.db.Close()
		return nil, err
	}

	// Load the prior persistance structures.
	err = h.load()
	if err != nil {
		_ = h.log.Close()
		_ = h.db.Close()
		return nil, err
	}

	// Get the host established on the network.
	err = h.initNetworking(address)
	if err != nil {
		_ = h.log.Close()
		_ = h.db.Close()
		return nil, err
	}

	return h, nil
}

// New returns an initialized Host.
func New(cs modules.ConsensusSet, tpool modules.TransactionPool, wallet modules.Wallet, address string, persistDir string) (*Host, error) {
	return newHost(productionPersister{}, cs, tpool, wallet, address, persistDir)
}

// Close shuts down the host, preparing it for garbage collection.
func (h *Host) Close() error {
	// Unsubscribe the host from the consensus set. Call will not terminate
	// until the last consensus update has been sent to the host.
	// Unsubscription must happen before any resources are released or
	// terminated because the process consensus change function makes use of
	// those resources.
	h.cs.Unsubscribe(h)

	// Close the listener, which means incoming network connections will be
	// rejected. The listener should be closed before the host resources are
	// disabled, as incoming connections will want to use the hosts resources.
	err := h.listener.Close()
	if err != nil {
		return err
	}

	// Close the bolt database.
	err = h.db.Close()
	if err != nil {
		return err
	}

	// Grab the resource lock and indicate that the host is closing. Concurrent
	// functions hold the resource lock until they terminate, meaning that no
	// threaded function will be running by the time the resource lock is
	// acquired.
	h.resourceLock.Lock()
	h.closed = true
	h.resourceLock.Unlock()

	// Clear the port that was forwarded at startup. The port handling must
	// happen before the logger is closed, as it leaves a logging message.
	err = h.clearPort(h.netAddress.Port())
	if err != nil {
		return err
	}

	// Save the latest host state.
	h.mu.Lock()
	err = h.save()
	h.mu.Unlock()
	if err != nil {
		return err
	}

	// Close the logger. The logger should be the last thing to shut down so
	// that all other objects have access to logging while closing.
	err = h.log.Close()
	if err != nil {
		return err
	}
	return nil
}
