// Package host is an implementation of the host module, and is responsible for
// storing data and advertising available storage to the network.
package host

import (
	"crypto/rand"
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

// TODO: Write a document on how to use the host, suggest that RAID0 be used to
// protect the core files from drive failure, emphasize that the host is pretty
// relient on having information that is not outdated.

const (
	// defaultMaxDuration defines the maximum number of blocks into the future
	// that the host will accept for the duration of an incoming file contract
	// obligation. 6 months is chosen because hosts are expected to be
	// long-term entities, and because we want to have a set of hosts that
	// support 6 month contracts when Sia leaves beta.
	defaultMaxDuration = 144 * 30 * 6 // 6 months.

	// maximumStorageFolders indicates the maximum number of storage folders
	// that the host allows. Some operations, such as creating a new storage
	// folder, take longer if there are more storage folders. Static RAM usage
	// also increases as the number of storage folders increase. For this
	// reason, a limit on the maximum number of storage folders has been set.
	maximumStorageFolders = 100

	// resubmissionTimeout defines the number of blocks that a host will wait
	// before attempting to resubmit a transaction to the blockchain.
	// Typically, this transaction will contain either a file contract, a file
	// contract revision, or a storage proof.
	resubmissionTimeout = 3

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

	// defaultContractPrice defines the default price of creating a contract
	// with the host. The default is set to 50 siacoins, which means that the
	// opening file contract can have 5 siacoins put towards it, the file
	// contract revision can have 15 siacoins put towards it, and the storage
	// proof can have 15 siacoins put towards it, with 5 left over for the
	// host, a sort of compensation for keeping and backing up the storage
	// obligation data.
	defaultContractPrice = types.NewCurrency64(50).Mul(types.SiacoinPrecision) // 50 siacoins

	// defaultUploadBandwidthPrice defines the default price of upload
	// bandwidth. The default is set to 1 siacoin per GB, because the host is
	// presumed to have a large amount of downstream bandwidth. Furthermore,
	// the host is typically only downloading data if it is planning to store
	// the data, meaning that the host serves to profit from accepting the
	// data.
	defaultUploadBandwidthPrice = modules.BandwidthPriceToConsensus(1e3) // 1 SC / GB

	// defaultDownloadBandwidthPrice defines the default price of upload
	// bandwidth. The default is set to 10 siacoins per gigabyte, because
	// download bandwidth is expected to be plentiful but also in-demand.
	defaultDownloadBandwidthPrice = modules.BandwidthPriceToConsensus(10e3) // 10 SC / GB

	// defaultStoragePrice defines the starting price for hosts selling
	// storage. We try to match a number that is both reasonably profitable and
	// reasonably competitive.
	defaultStoragePrice = modules.StoragePriceToConsensus(20e3) // 20 SC / GB / Month

	// defaultCollateral defines the amount of money that the host puts up as
	// collateral per-byte by default. The collateral should be considered as
	// an absolute instead of as a percentage, because low prices result in
	// collaterals which may be significant by percentage, but insignificant
	// overall. A default of 50 SC / GB / Month has been chosen, which is 2.5x
	// the default price for storage. The host is expected to put up a
	// significant amount of collateral as a commitment to faithfulness,
	// because this guarantees that the incentives are aligned for the host to
	// keep the data even if the price of siacoin fluctuates, the price of raw
	// storage fluctuates, or the host realizes that there is unexpected
	// opportunity cost in being a host.
	defaultCollateral = types.NewCurrency64(50) // 50 SC / GB / Month

	// defaultWindowSize is the size of the proof of storage window requested
	// by the host. The host will not delete any obligations until the window
	// has closed and buried under several confirmations. For release builds,
	// the default is set to 144 blocks, or about 1 day. This gives the host
	// flexibility to experience downtime without losing file contracts. The
	// optimal default, especially as the network matures, is probably closer
	// to 36 blocks. An experienced or high powered host should not be
	// frustrated by lost coins due to long periods of downtime.
	defaultWindowSize = func() types.BlockHeight {
		if build.Release == "dev" {
			return 36 // 3.6 minutes.
		}
		if build.Release == "standard" {
			return 144 // 1 day.
		}
		if build.Release == "testing" {
			return 5 // 5 seconds.
		}
		panic("unrecognized release constant in host - defaultWindowSize")
	}()

	// maximumStorageFolderSize sets an upper bound on how large storage
	// folders in the host are allowed to be. It makes sure that inputs and
	// constructions are sane. While it's conceivable that someone could create
	// a rig with a single logical storage folder greater than 128 TiB in size
	// in production, it's probably not a great idea, especially when you are
	// allowed to use many storage folders. All told, a single host on today's
	// constants can support up to ~10 PB of storage.
	maximumStorageFolderSize = func() uint64 {
		if build.Release == "dev" {
			return 1 << 33 // 8 GiB
		}
		if build.Release == "standard" {
			return 1 << 47 // 128 TiB
		}
		if build.Release == "testing" {
			return 1 << 20 // 1 MiB
		}
		panic("unrecognized release constant in host - maximum storage folder size")
	}()

	// maximumVirtualSectors defines the maximum number of virtual sectors that
	// can be tied to each physical sector.
	maximumVirtualSectors = func() int {
		if build.Release == "dev" {
			// The testing value is at 35 to provide flexibility. The
			// development value is at 5 because hitting the virtual sector
			// limit in a sane development environment is more difficult than
			// hitting the virtual sector limit in a controlled testing
			// environment (dev environment doesn't have access to private
			// methods such as 'addSector'.
			return 5
		}
		if build.Release == "standard" {
			// Each virtual sector adds about 8 bytes of load to the host
			// persistence structures, and additionally adds 8 bytes of load
			// when reading or modifying a sector. Though a few virtual sectors
			// with 10e3 or even 100e3 virtual sectors would not be too
			// detrimental to the host, tens of thousands of physical sectors
			// that each have ten thousand virtual sectors could pose a problem
			// for the host. In most situations, a renter will not need more 2
			// or 3 virtual sectors when manipulating data, so 250 is generous
			// as long as the renter is properly encrypting data. 250 is
			// unlikely to cause the host problems, even if an attacker is
			// creating hundreds of thousands of phsyical sectors (an expensive
			// action!) each with 250 vitrual sectors.
			return 250
		}
		if build.Release == "testing" {
			return 35
		}
		panic("unrecognized release constant in host - maximum virtual sector size")
	}()

	// minimumStorageFolderSize defines the smallest size that a storage folder
	// is allowed to be. The new design of the storage folder structure means
	// that this limit is not as relevant as it was originally, but hosts with
	// little storage capacity are not very useful to the network, and can
	// actually frustrate price planning. 32 GB has been chosen as a minimum
	// for the early days of the network, to allow people to experiment in the
	// beta, but in the future I think something like 256GB would be much more
	// appropraite.
	minimumStorageFolderSize = func() uint64 {
		if build.Release == "dev" {
			return 1 << 25 // 32 MiB
		}
		if build.Release == "standard" {
			return 1 << 35 // 32 GiB
		}
		if build.Release == "testing" {
			return 1 << 15 // 32 KiB
		}
		panic("unrecognized release constant in host - minimum storage folder size")
	}()

	// sectorSize defines how large a sector should be in bytes. The sector
	// size needs to be a power of two to be compatible with package
	// merkletree. 4MB has been chosen for the live network because large
	// sectors significantly reduce the tracking overhead experienced by the
	// renter and the host.
	sectorSize = func() uint64 {
		if build.Release == "dev" {
			return 1 << 20 // 1 MiB
		}
		if build.Release == "standard" {
			return 1 << 22 // 4 MiB
		}
		if build.Release == "testing" {
			return 1 << 12 // 4 KiB
		}
		panic("unrecognized release constant in host - sectorSize")
	}()

	// storageFolderUIDSize determines the number of bytes used to determine
	// the storage folder UID. Production and development environments use 4
	// bytes to minimize the possibility of accidental collisions, and testing
	// environments use 1 byte so that collisions can be forced while using the
	// live code.
	storageFolderUIDSize = func() int {
		if build.Release == "dev" {
			return 2
		}
		if build.Release == "standard" {
			return 4
		}
		if build.Release == "testing" {
			return 1
		}
		panic("unrecognized release constant in host - storageFolderUIDSize")
	}()

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
	// bucketActionItems maps a blockchain height to a list of storage
	// obligations that need to be managed in some way at that height. The
	// height is stored as a big endian uint64, which means that bolt will
	// store the heights sorted in numerical order. The action item itself is
	// an array of file contract ids. The host is able to contextually figure
	// out what the necessary actions for that item are based on the file
	// contract id and the associated storage obligation that can be retreived
	// using the id.
	bucketActionItems = []byte("BucketActionItems")

	// bucketSectorUsage maps sector IDs to the number of times they are used
	// in file contracts. If all data is correctly encrypted using a unique
	// seed, each sector will be in use exactly one time. The host however
	// cannot control this, and a user may upload unencrypted data or
	// intentionally upload colliding sectors as a means of attack. The host
	// can only delete a sector when it is in use zero times. The number of
	// times a sector is in use is encoded as a big endian uint64.
	bucketSectorUsage = []byte("BucketSectorUsage")

	// bucketStorageObligations contains a set of serialized
	// 'storageObligations' sorted by their file contract id.
	bucketStorageObligations = []byte("BucketStorageObligations")
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

	// Dependencies.
	cs     modules.ConsensusSet
	tpool  modules.TransactionPool
	wallet modules.Wallet
	dependencies

	// Consensus Tracking.
	blockHeight  types.BlockHeight
	recentChange modules.ConsensusChangeID

	// Host Identity
	netAddress modules.NetAddress
	publicKey  types.SiaPublicKey
	secretKey  crypto.SecretKey
	sectorSalt crypto.Hash

	// Storage Obligation Management - different from file management in that
	// the storage obligation management is the new way of handling storage
	// obligations. Is a replacement for the contract obligation logic, but the
	// old logic is being kept for compatibility purposes.
	//
	// Storage is broken up into sectors. The sectors are distributed across a
	// set of storage folders using a strategy that tries to create even
	// distributions, but not aggressively. Uneven distributions could be
	// manufactured by an attacker given sufficent knowledge about the disk
	// layout (knowledge which should be unavailable), but a limited amount of
	// damage can be done even with this attack.
	//
	// TODO: lockedStorageObligations is currently unbounded. A safety needs to
	// be added that makes sure the number of simultaneous locked obligations
	// stays below 5e3.
	lockedStorageObligations map[types.FileContractID]struct{} // Which storage obligations are currently being modified.
	storageFolders           []*storageFolder

	// Statistics
	anticipatedRevenue types.Currency
	lostRevenue        types.Currency
	revenue            types.Currency

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
}

// establishDefaults configures the default settings for the host, overwriting
// any existing settings.
func (h *Host) establishDefaults() error {
	// Configure the settings object.
	h.settings = modules.HostSettings{
		MaxDuration: defaultMaxDuration,
		WindowSize:  defaultWindowSize,

		Collateral:             defaultCollateral,
		ContractPrice:          defaultContractPrice,
		DownloadBandwidthPrice: defaultDownloadBandwidthPrice,
		UploadBandwidthPrice:   defaultUploadBandwidthPrice,
	}

	// Generate signing key, for revising contracts.
	sk, pk, err := crypto.GenerateKeyPair()
	if err != nil {
		return err
	}
	h.secretKey = sk
	h.publicKey = types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       pk[:],
	}
	_, err = rand.Read(h.sectorSalt[:])
	if err != nil {
		return err
	}

	// Subscribe to the consensus set.
	return h.initConsensusSubscription()
}

// initDB will check that the database has been initialized and if not, will
// initialize the database.
func (h *Host) initDB() error {
	return h.db.Update(func(tx *bolt.Tx) error {
		// Return nil if the database is already initialized. The database can
		// be safely assumed to be initialized if the storage obligation bucket
		// exists.
		bso := tx.Bucket(bucketStorageObligations)
		if bso != nil {
			return nil
		}

		// The storage obligation bucket does not exist, which means the
		// database needs to be initialized. Create the database buckets.
		buckets := [][]byte{
			bucketActionItems,
			bucketSectorUsage,
			bucketStorageObligations,
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
func newHost(dependencies dependencies, cs modules.ConsensusSet, tpool modules.TransactionPool, wallet modules.Wallet, address string, persistDir string) (*Host, error) {
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
		cs:           cs,
		tpool:        tpool,
		wallet:       wallet,
		dependencies: dependencies,

		// actionItems: make(map[types.BlockHeight]map[types.FileContractID]*contractObligation),

		// obligationsByID: make(map[types.FileContractID]*contractObligation),

		lockedStorageObligations: make(map[types.FileContractID]struct{}),

		persistDir: persistDir,
	}

	// Create the perist directory if it does not yet exist.
	err := dependencies.mkdirAll(h.persistDir, 0700)
	if err != nil {
		return nil, err
	}

	// Initialize the logger. Logging should be initialized ASAP, because the
	// rest of the initialization makes use of the logger.
	h.log, err = dependencies.newLogger(filepath.Join(h.persistDir, logFile))
	if err != nil {
		return nil, err
	}

	// Open the database containing the host's storage obligation metadata.
	h.db, err = dependencies.openDatabase(dbMetadata, filepath.Join(h.persistDir, dbFilename))
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
	return newHost(productionDependencies{}, cs, tpool, wallet, address, persistDir)
}

// composeErrors will take two errors and compose them into a single errors
// with a longer message. Any nil errors used as inputs will be stripped out,
// and if there are zero non-nil inputs then 'nil' will be returned.
//
// TODO: It may make sense to move this function to the build package. When
// moving it, the testing function should follow.
func composeErrors(errs ...error) error {
	// Strip out any nil errors.
	var filteredErrs []error
	for _, err := range errs {
		if err != nil {
			filteredErrs = append(filteredErrs, err)
		}
	}

	// Return nil if there are no non-nil errors in the input.
	if len(filteredErrs) <= 0 {
		return nil
	}

	// Combine all of the non-nil errors into one larger return value.
	err := filteredErrs[0]
	for i := 1; i < len(filteredErrs); i++ {
		err = errors.New(err.Error() + " and " + filteredErrs[i].Error())
	}
	return err
}

// Close shuts down the host, preparing it for garbage collection.
func (h *Host) Close() (composedError error) {
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
		composedError = composeErrors(composedError, err)
	}

	// Grab the resource lock and indicate that the host is closing. Concurrent
	// functions hold the resource lock until they terminate, meaning that no
	// threaded function will be running by the time the resource lock is
	// acquired.
	h.resourceLock.Lock()
	h.closed = true
	h.resourceLock.Unlock()

	// Close the bolt database.
	err = h.db.Close()
	if err != nil {
		composedError = composeErrors(composedError, err)
	}

	// Clear the port that was forwarded at startup. The port handling must
	// happen before the logger is closed, as it leaves a logging message.
	err = h.clearPort(h.netAddress.Port())
	if err != nil {
		composedError = composeErrors(composedError, err)
	}

	// Save the latest host state.
	h.mu.Lock()
	err = h.save()
	h.mu.Unlock()
	if err != nil {
		composedError = composeErrors(composedError, err)
	}

	// Close the logger. The logger should be the last thing to shut down so
	// that all other objects have access to logging while closing.
	err = h.log.Close()
	if err != nil {
		composedError = composeErrors(composedError, err)
	}
	return composedError
}
