package contractmanager

import (
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/persist"
	siasync "github.com/NebulousLabs/Sia/sync"
)

// ContractManager is responsible for managing contracts that the host has with
// renters, including storing the data, submitting storage proofs, and deleting
// the data when a contract is complete.
//
// The contract manager controls many resources which are spread across
// multiple files yet must all be consistent and durable. Both have been
// achieved by using write-ahead-logging and ACID transactions. The state of
// the contract manager object itself contains only what has happened in
// updates which have been fully committed - the contract manager is not
// updated unless the changes for the update have been fully committed to all
// resources which adjust according to the changes.
//
// The memory management and concurrency model of the contract manager is
// pretty sharply different from that of other modules, as there is a lot of
// I/O, much of which is happening on slow/cheap HDDs, yet performance is
// critical.
//
// All operations that mutate the contract manager (except for during
// initialization) go through a write-ahead-logger (WAL). The WAL manages most
// of the locking and blockhing that happens when external modules are
// interacting with the contract manager.
type ContractManager struct {
	// Storage management information.
	//
	// TODO: explain that the sector salt is necessary to reduce the internal
	// names for the sectors from 32bytes to just 12 bytes.
	sectorSalt     crypto.Hash
	storageFolders []*storageFolder

	// In-memory representation of the sector location lookups which are kept
	// on disk. This representation is kept in-memory so that efficient
	// constant-time-lookups can be used to figure out where sectors are stored
	// on disk. This prevents I/O from being a significant bottleneck. 10TiB of
	// data stored on the host will bloat the map to about 1.5GiB in size, and
	// the map will be able to support millions of reads and writes per second.
	sectorLocations map[string]sectorLocation

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
	mu         sync.RWMutex
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
		sectorLocations: make(map[string]sectorLocation),

		dependencies: dependencies,
		persistDir:   persistDir,
	}
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

	// Load any persistent state of the contract manager from disk.
	err = cm.load()
	if err != nil {
		return nil, build.ExtendErr("error while performing contract manager load", err)
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
