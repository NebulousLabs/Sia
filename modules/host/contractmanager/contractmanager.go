package contractmanager

// TODO: Currently, we don't do any directory Sync'ing following COW
// operations, that may be necessary to provide strong guarantees against data
// corruption.

// TODO: The writeaheadlog is misusing log.Critical - it's using log.Critical
// to indicate that there are severe problems with the host, but these are not
// developer issues, they are likely disk issues. Instead of log.Critical,
// there should be a log.Crash. The program should be crashing regardless of
// DEBUG mode, which is why the logging statements are followed by a bunch of
// panics. Extending the logger could clean up this code some.

// TODO: There might be a smarter way to manage the sync interval. Some syncs
// are going to take several seconds, and some are going to take almost no time
// at all. The goal is to maximize throughput, but it's also important that the
// host be responsive when accepting changes from the renter. Batched calls on
// the renter side can improve throughput despite 1.5 second latencies
// per-change, but that's basically requiring the renter to add code complexity
// in order to achieve throughputs greater than 4 MiB every 1.5 seconds.
//
// It should be noted that if the renter is properly doing parallel uploads, a
// throughput of 10+ mbps per host is pretty decent, even without batching.

// TODO: In managedAddStorageFolder, fallocate can be used instead of
// file.Write, which means that storage folders can be added substantially
// faster. Windows and other non-linux systems will need to continue doing it
// using the current implementation.

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/persist"
	siasync "github.com/NebulousLabs/Sia/sync"
)

// ContractManager is responsible for managing contracts that the host has with
// renters, including storing the data, submitting storage proofs, and deleting
// the data when a contract is complete.
type ContractManager struct {
	// The contract manager controls many resources which are spread across
	// multiple files yet must all be consistent and durable. ACID properties
	// have been achieved by using a write-ahead-logger (WAL). All operations
	// that read or mutate the stateful fields of the contract manager must go
	// through the WAL or inconsistency is risked.
	//
	// The state of the contract manager is broken up into two separate
	// categories. The first category is atomic state, and the second category
	// is non-atomic state. Fields like the sector salt and storage folders are
	// always saved to disk atomically using a copy-on-write technique. Sector
	// location information and sector data are written directly to a large
	// file, without using copy-on-write, which means that the writes are
	// non-atomic.
	//
	// Each category of state must be treated differently. The non-atomic data
	// is formatted very cleanly - every field on disk has an exact size, which
	// means that the WAL can write idempotent commits that specify the exact
	// location on disk where a write should happen to successfully execute an
	// action. The atomic state contains fields with variable sizes, and
	// therefore must be approached differently.
	//
	// The exact-location writing style of the disk can be mimiced by pulling
	// all of the variable size state into memory, and then indexing it using
	// explit key-value pairs.
	//
	// The in-progress updates in the WAL itself should all be idempotent, and
	// constructed in a way such that they do not rely on any state at all to
	// execute properly, as the current state may be inconsistent or unknown.

	// In-memory representation of the sector location lookups which are kept
	// on disk. This representation is kept in-memory so that efficient
	// constant-time-lookups can be used to figure out where sectors are stored
	// on disk. This prevents I/O from being a significant bottleneck. 10TiB of
	// data stored on the host will bloat the map to about 1.5GiB in size, and
	// the map will be able to support millions of reads and writes per second.
	//
	// folderLocations contains a mapping from storage folder indexes to the
	// on-disk location of that storage folder. The folderLocations object can
	// be updated before commitments are made without losing durability,
	// because the folderLocations object does not get saved to disk directly,
	// instead its status is inferred entirely at startup.
	sectorSalt      crypto.Hash
	sectorLocations map[sectorID]sectorLocation
	storageFolders  map[uint16]*storageFolder

	// Utilities. The dependencies are package or filesystem dependencies, and
	// are provided so that the dependencies can be mocked during testing. Sia
	// is generally not a very mock-heavy project, but the ACID requirements of
	// the contract manager mean that a lot of logic will only occur following
	// a disk failure or some other unexpected failure, and in testing we need
	// to be able to mock these failures.
	//
	// The mutex protects all of the stateful fields of the contract manager,
	// and should be used any time that one of the fields is being accessed or
	// modified.
	//
	// The WAL is responsible for all of the mutation within the contract
	// manager, and is generally the only one accessing the contract manager's
	// mutex.
	dependencies
	log        *persist.Logger
	persistDir string
	tg         siasync.ThreadGroup
	wal        writeAheadLog
}

// Close will cleanly shutdown the contract manager, closing all resources and
// goroutines that are in use, blocking until shutdown has completed.
func (cm *ContractManager) Close() error {
	return build.ExtendErr("error while stopping contract manager", cm.tg.Stop())
}

// newContrctManager returns a contract manager that is ready to be used with
// the provided dependencies.
func newContractManager(dependencies dependencies, persistDir string) (*ContractManager, error) {
	cm := &ContractManager{
		storageFolders:  make(map[uint16]*storageFolder),
		sectorLocations: make(map[sectorID]sectorLocation),

		dependencies: dependencies,
		persistDir:   persistDir,
	}
	// Because the wal and the cm each have eachother as objects in their
	// structs, they really operate as a single larger object. When designing
	// and implementing, I found it easier to reason about each when keeping
	// their functions separate, though they rely heavily on eachother.
	//
	// The wal is used any time that the state of the cm is read or modified.
	// The cm is just a record of how things currently look. A future refactor
	// would probably combine the structs into a single object.
	cm.wal.cm = cm

	// If startup is unsuccessful, shutdown any resources that were
	// successfully spun up.
	var err error
	defer func() {
		if err != nil {
			err1 := build.ExtendErr("error during contract manager startup", err)
			err2 := build.ExtendErr("error while stopping a partially started contract manager", cm.tg.Stop())
			err = build.ComposeErrors(err1, err2)
		}
	}()

	// Create the perist directory if it does not yet exist.
	err = dependencies.mkdirAll(cm.persistDir, 0700)
	if err != nil {
		return nil, build.ExtendErr("error while creating the persist directory for the contract manager", err)
	}

	// Initialize the logger. Logging should be initialized ASAP, because the
	// rest of the initialization makes use of the logger.
	cm.log, err = dependencies.newLogger(filepath.Join(cm.persistDir, logFile))
	if err != nil {
		return nil, build.ExtendErr("error while creating the logger for the contract manager", err)
	}
	// Set up the clean shutdown of the logger.
	cm.tg.AfterStop(func() {
		err = build.ComposeErrors(cm.log.Close(), err)
	})

	// Load any state of the contract manager that gets saved atomically.
	// Changes that were incompletely applied will not be represented as
	// corruption, because the data is copied atomically. Instead, they will be
	// represented as absent altogether. When the WAL is recovered, the absent
	// changes will be reapplied.
	err = cm.loadAtomicPersistence()
	if err != nil {
		return nil, build.ExtendErr("error while loading contract manager atomic data", err)
	}

	// Allow the WAL to load. The WAL will check for an unclean shutdown, make
	// any repairs necessary following the unclean shutdown, and then establish
	// the sync loop and sync resources that are used to maintain ACID
	// properties as the contact manager state is manipulated.
	err = cm.wal.load()
	if err != nil {
		return nil, build.ExtendErr("error while loading the WAL at startup", err)
	}

	// Spin up the sync loop. Note that the sync loop needs to be created after
	// the loading process is complete, otherwise there might be conflicts on
	// the contract state, as commit() will be modifying the state and saving
	// things to disk.
	err = cm.wal.spawnSyncLoop()
	if err != nil {
		return nil, build.ExtendErr("error while spawning contract manager sync loop", err)
	}
	return cm, nil
}

// New returns a new ContractManager.
func New(persistDir string) (*ContractManager, error) {
	return newContractManager(productionDependencies{}, persistDir)
}
