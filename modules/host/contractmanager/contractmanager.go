package contractmanager

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/persist"
	siasync "github.com/NebulousLabs/Sia/sync"
)

// CM isa
//
// Content represents everything excluding events in the committed and
// uncommitted WAL.
type ContractManager struct {
	// Storage management information.
	//
	// TODO: explain that the sector salt is necessary to reduce the internal
	// names for the sectors from 32bytes to just 12 bytes.
	sectorLocations map[string]sectorLocation
	sectorSalt      crypto.Hash
	storageFolders  []*storageFolder

	// Utilities.
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
	return cm.tg.Stop()
}

// newContrctManager returns a contract manager that is ready to be used with
// the provided dependencies.
func newContractManager(dependencies dependencies, persistDir string) (*ContractManager, error) {
	cm := &ContractManager{
		dependencies: dependencies,

		sectorLocations: make(map[string]sectorLocation),

		persistDir: persistDir,
	}
	cm.wal.cm = cm

	// If startup is unsuccessful, shutdown any resources that were
	// successfully spun up.
	var err error
	defer func() {
		if err != nil {
			err = composeErrors(cm.tg.Stop(), err)
		}
	}()

	// Create the perist directory if it does not yet exist.
	err = dependencies.mkdirAll(cm.persistDir, 0700)
	if err != nil {
		return nil, err
	}

	// Initialize the logger. Logging should be initialized ASAP, because the
	// rest of the initialization makes use of the logger.
	cm.log, err = dependencies.newLogger(filepath.Join(cm.persistDir, logFile))
	if err != nil {
		return nil, err
	}
	// Set up the clean shutdown of the logger.
	cm.tg.AfterStop(func() {
		err = cm.log.Close()
		if err != nil {
			// State of the logger is uncertain, a Println will have to
			// suffice.
			fmt.Println("Error closing the contract manager logger:", err)
		}
	})

	// Load any persistent state of the contract manager from disk.
	err = cm.load()
	if err != nil {
		return nil, err
	}
	return cm, nil
}

// New returns a new ContractManager.
func New(persistDir string) (*ContractManager, error) {
	return newContractManager(productionDependencies{}, persistDir)
}
