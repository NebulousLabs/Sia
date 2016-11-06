package contractmanager

// TODO: Currently, we don't do any directory syncs after atomic save-then-move
// file operations, that may be necessary to provide strong guarantees against
// data corruption.

// TODO: In managedAddStorageFolder, fallocate can be used instead of
// file.Write, which means that storage folders can be added substantially
// faster. Windows and other non-linux systems will need to continue doing it
// using the current implementation.

// TODO: Currently the long running storage folder operations are expected to
// have their progress value's menaing determined by context, but that's really
// only possible with the WAL, which external callers cannot view. Explicit
// context should be added to the struct.

// TODO: Need per-storage-folder locking so that sector writes and seeks don't
// conflict with eachother in the file handle.

// TODO: Set up a stress test across multiple disks that verifies the contract
// manager is able to hit a throughput which is fully utilizing all of the
// disks, or at least is coming close. The goal is to catch unexpected
// performance bottlenecks.

// TODO: Because of the naive balancing method, one slow disk will slow down
// the whole host until it's at capacity, because it'll continually be selected
// as the emptiest storage folder even though it might already be under max
// load and other folders might be idle.

// TODO: The empitest storage folder might not be calculated by looking at the
// usage, which is the important metric to be using for this purpose. We can
// probably fix this up by adding functions to manipulate the usage field.

// TODO: Perform a test simulating a multi-disk environment where after a
// restart one of the disks is unavailable.

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
	// have been achieved by using a write-ahead-logger (WAL). The in-memory
	// state represents currently uncommitted data, however reading from the
	// uncommitted state does not threaten consistency. It is okay if the user
	// sees uncommitted data, so long as other ACID operations do not return
	// early. Any changes to the state must be documented in the WAL to prevent
	// inconsistency.

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

	// lockedSectors contains a list of sectors that are currently being read
	// or modified.
	lockedSectors map[sectorID]*sectorLock

	// Utilities.
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
	err = cm.loadSettings()
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
