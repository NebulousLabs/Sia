package contractmanager

// TODO: Need to sync the directory after doing rename and create operations.

// TODO: Use fallocate when adding + growing storage folders.

// TODO: Long-running operations (add, empty) don't tally progress, and don't
// indicate what operation is running.

// TODO: Add disk failure testing.

// TODO: Write some code into the production dependencies that will, during
// testing, arbitrarily write less than the full data to a file until Sync()
// has been called. That way, disruptions can effectively simulate partial
// writes even though the disk writes are actually completing.

// TODO: emptyStorageFolder should be able to move sectors into folders that
// are being resized, into the sectors that are not affected by the resize.

// TODO: Re-write the WAL to not need to do group syncing, and also to not need
// to use the rename call at all.

// TODO: When a storage folder is missing, operations on the sectors in that
// storage folder (Add, Remove, Delete, etc.) may result in corruption and
// inconsistent internal state for the contractor. For now, this is fine because
// it's a rare situation, but it should be addressed eventually.

import (
	"errors"
	"path/filepath"
	"sync/atomic"

	"gitlab.com/NebulousLabs/Sia/build"
	"gitlab.com/NebulousLabs/Sia/crypto"
	"gitlab.com/NebulousLabs/Sia/modules"
	"gitlab.com/NebulousLabs/Sia/persist"
	siasync "gitlab.com/NebulousLabs/Sia/sync"
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

	// The contract manager is highly concurrent. Most fields are protected by
	// the mutex in the WAL, but storage folders and sectors can be accessed
	// individually. A map of locked sectors ensures that each sector is only
	// accessed by one thread at a time, but allows many sectors across a
	// single file to be accessed concurrently. Any interaction with a sector
	// requires a sector lock.
	//
	// If sectors are being added to a storage folder, a readlock is required
	// on the storage folder. Reads and deletes do not require any locks on the
	// storage folder. If a storage folder operation is happening (add, resize,
	// remove), a writelock is required on the storage folder lock.

	// The contract manager is expected to be consistent, durable, atomic, and
	// error-free in the face of unclean shutdown and disk error. Failure of
	// the controlling disk (containing the settings file and WAL file) is not
	// tolerated and will cause a panic, but any disk failures for the storage
	// folders should be tolerated gracefully. Threads should perform complete
	// cleanup before returning, which can be achieved with threadgroups.

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
	dependencies modules.Dependencies
	log          *persist.Logger
	persistDir   string
	tg           siasync.ThreadGroup
	wal          writeAheadLog
}

// Close will cleanly shutdown the contract manager.
func (cm *ContractManager) Close() error {
	return build.ExtendErr("error while stopping contract manager", cm.tg.Stop())
}

// newContractManager returns a contract manager that is ready to be used with
// the provided dependencies.
func newContractManager(dependencies modules.Dependencies, persistDir string) (*ContractManager, error) {
	cm := &ContractManager{
		storageFolders:  make(map[uint16]*storageFolder),
		sectorLocations: make(map[sectorID]sectorLocation),

		lockedSectors: make(map[sectorID]*sectorLock),

		dependencies: dependencies,
		persistDir:   persistDir,
	}
	cm.wal.cm = cm
	cm.tg.AfterStop(func() {
		dependencies.Destruct()
	})

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
	err = dependencies.MkdirAll(cm.persistDir, 0700)
	if err != nil {
		return nil, build.ExtendErr("error while creating the persist directory for the contract manager", err)
	}

	// Logger is always the first thing initialized.
	cm.log, err = dependencies.NewLogger(filepath.Join(cm.persistDir, logFile))
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
		cm.log.Println("ERROR: Unable to load contract manager settings:", err)
		return nil, build.ExtendErr("error while loading contract manager atomic data", err)
	}

	// Load the WAL, repairing any corruption caused by unclean shutdown.
	err = cm.wal.load()
	if err != nil {
		cm.log.Println("ERROR: Unable to load the contract manager write-ahead-log:", err)
		return nil, build.ExtendErr("error while loading the WAL at startup", err)
	}
	// Upon shudown, unload all of the files.
	cm.tg.AfterStop(func() {
		cm.wal.mu.Lock()
		defer cm.wal.mu.Unlock()

		for _, sf := range cm.storageFolders {
			// No storage folder to close if the folder is not available.
			if atomic.LoadUint64(&sf.atomicUnavailable) == 1 {
				// File handles will either already be closed or may even be
				// nil.
				continue
			}

			err = sf.metadataFile.Close()
			if err != nil {
				cm.log.Println("Error closing the storage folder file handle", err)
			}
			err = sf.sectorFile.Close()
			if err != nil {
				cm.log.Println("Error closing the storage folder file handle", err)
			}
		}
	})

	// The sector location data is loaded last. Any corruption that happened
	// during unclean shutdown has already been fixed by the WAL.
	for _, sf := range cm.storageFolders {
		if atomic.LoadUint64(&sf.atomicUnavailable) == 1 {
			// Metadata unavailable, just count the number of sectors instead of
			// loading them.
			sf.sectors = uint64(len(usageSectors(sf.usage)))
			continue
		}
		cm.loadSectorLocations(sf)
	}

	// Launch the sync loop that periodically flushes changes from the WAL to
	// disk.
	err = cm.wal.spawnSyncLoop()
	if err != nil {
		cm.log.Println("ERROR: Unable to spawn the contract manager synchronization loop:", err)
		return nil, build.ExtendErr("error while spawning contract manager sync loop", err)
	}

	// Spin up the thread that continuously looks for missing storage folders
	// and adds them if they are discovered.
	go cm.threadedFolderRecheck()

	// Simulate an error to make sure the cleanup code is triggered correctly.
	if cm.dependencies.Disrupt("erroredStartup") {
		err = errors.New("startup disrupted")
		return nil, err
	}
	return cm, nil
}

// New returns a new ContractManager.
func New(persistDir string) (*ContractManager, error) {
	return newContractManager(new(modules.ProductionDependencies), persistDir)
}
