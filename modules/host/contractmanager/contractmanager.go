package contractmanager

// TODO: Currently, we don't do any directory syncs after atomic save-then-move
// file operations, that may be necessary to provide strong guarantees against
// data corruption.

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

// TODO: Some of the locking, especially with regards to ReadSector, could be
// moved to a per-storage-folder basis instead of grabbing the WAL lock, which
// blocks everything to access just a single resource.

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
	// always saved to disk atomically. Sector location information and sector
	// data are written directly to a large file, without using copy-on-write,
	// which means that the writes are non-atomic.
	//
	// Atomic writes can be handled using the write-then-rename technique for
	// the files. This works for small fields that don't update frequently.
	//
	// Non-atomic data is managed by combining idempotency with a WAL. The WAL
	// can specify 'write data X to file Y, offset Z', such that in the event
	// of power loss, following those instructions again will always restore
	// consistency.

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

	// sectorSalt is a persistent security field that gets set the first time
	// the contract manager is initiated and then never gets touched again.
	// It's used to randomize the location on-disk that a sector gets stored,
	// so that an adversary cannot maliciously add sectors to specific disks,
	// or otherwise perform manipulations that may degrade performance.
	//
	// sectorLocations is a giant lookup table that keeps a mapping from every
	// sector in the host to the location on-disk where it is stored. For
	// performance information, see the BenchmarkSectorLocations docstring.
	// sectorLocations is persisted on disk through a combination of the WAL
	// and through metadata that is stored directly in each storage folder.
	//
	// The storageFolders fields stores information about each storage folder,
	// including metadata about which sector slots are currently populated vs.
	// which sector slots are available. For performance information, see
	// BenchmarkStorageFolders.
	sectorSalt      crypto.Hash
	sectorLocations map[sectorID]sectorLocation
	storageFolders  map[uint16]*storageFolder

	// Utilities.
	//
	// The WAL helps orchestrate complex ACID transactions within the contract
	// manager.
	dependencies
	log        *persist.Logger
	persistDir string
	tg         siasync.ThreadGroup
	wal        writeAheadLog
}

// Close will cleanly shutdown the contract manager.
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
	// The WAL and the contract manager have eachother in their structs,
	// meaning they are effectively one larger object. I find them easier to
	// reason about however by considering them as separate objects.
	cm.wal.cm = cm

	// Perform clean shutdown of already-initialized features if startup fails.
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

	// Logger is always the first thing initialized.
	cm.log, err = dependencies.newLogger(filepath.Join(cm.persistDir, logFile))
	if err != nil {
		return nil, build.ExtendErr("error while creating the logger for the contract manager", err)
	}
	// Set up the clean shutdown of the logger.
	cm.tg.AfterStop(func() {
		err = build.ComposeErrors(cm.log.Close(), err)
	})

	// Load the atomic state of the contract manager. Unclean shutdown may have
	// wiped out some changes that got made. Anything really important will be
	// recovered when the WAL is loaded.
	err = cm.loadAtomicPersistence()
	if err != nil {
		return nil, build.ExtendErr("error while loading contract manager atomic data", err)
	}

	// Load the WAL, repairing any corruption caused by unclean shutdown.
	err = cm.wal.load()
	if err != nil {
		return nil, build.ExtendErr("error while loading the WAL at startup", err)
	}
	// The contract manager should not be in a fully consistent state,
	// containing all transactions, data, and updates that it has externally
	// comitted to prior to the previous shutdown.

	// The sector location data is loaded last. Any corruption that happened
	// during unclean shutdown has already been fixed by the WAL.
	cm.loadSectorLocations()

	// Launch the sync loop that periodically flushes changes from the WAL to
	// disk.
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
