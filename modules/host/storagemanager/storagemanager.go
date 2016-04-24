// package storagemanager implements a storage manager for the host on Sia. The
// storage manager can add sectors, remove sectors, and manage how sectors are
// distributed between a number of storage folders.
package storagemanager

// TODO: The documentation still talks like this code is a part of the host.

import (
	"errors"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/persist"
)

const (
	// Names of the various persist files in the storage manager.
	dbFilename   = "storagemanager.db"
	logFile      = "storagemanager.log"
	settingsFile = "storagemanager.json"
)

var (
	// dbMetadata is the header that gets applied to the database to identify a
	// version and indicate what type of data is being stored in the database.
	dbMetadata = persist.Metadata{
		Header:  "Sia Storage Manager DB",
		Version: "0.6.0",
	}

	// persistMetadata is the header that gets added to the persist file to
	// identify the type of file and the version of the file.
	persistMetadata = persist.Metadata{
		Header:  "Sia Storage Manager",
		Version: "0.6.0",
	}

	errStorageManagerClosed = errors.New("call is disabled because storage manager is closed")
)

// StorageManager tracks multiple storage folders, and is responsible for
// adding sectors, removing sectors, and overall managing the way that data is
// stored.
type StorageManager struct {
	// Dependencies.
	dependencies

	// Storage management information.
	sectorSalt     crypto.Hash
	storageFolders []*storageFolder

	// Utilities.
	db         *persist.BoltDatabase
	log        *persist.Logger
	mu         sync.RWMutex
	persistDir string

	// The resource lock is held by threaded functions for the duration of
	// their operation. Functions should grab the resource lock as a read lock
	// unless they are planning on manipulating the 'closed' variable.
	// Readlocks are used so that multiple functions can use resources
	// simultaneously, but the resources are not closed until all functions
	// accessing them have returned.
	closed       bool
	resourceLock sync.RWMutex
}

// Close will shut down the storage manager.
func (sm *StorageManager) Close() (composedError error) {
	// Grab the resource lock and indicate that the host is closing. Concurrent
	// functions hold the resource lock until they terminate, meaning that no
	// threaded function will be running by the time the resource lock is
	// acquired.
	sm.resourceLock.Lock()
	closed := sm.closed
	sm.closed = true
	sm.resourceLock.Unlock()
	if closed {
		return nil
	}

	// Close the bolt database.
	err := sm.db.Close()
	if err != nil {
		composedError = composeErrors(composedError, err)
	}

	// Save the latest host state.
	sm.mu.Lock()
	err = sm.save()
	sm.mu.Unlock()
	if err != nil {
		composedError = composeErrors(composedError, err)
	}

	// Close the logger. The logger should be the last thing to shut down so
	// that all other objects have access to logging while closing.
	err = sm.log.Close()
	if err != nil {
		composedError = composeErrors(composedError, err)
	}
	return composedError
}

// newStorageManager creates a new storage manager.
func newStorageManager(dependencies dependencies, persistDir string) (*StorageManager, error) {
	sm := &StorageManager{
		dependencies: dependencies,

		persistDir: persistDir,
	}

	// Create the perist directory if it does not yet exist.
	err := dependencies.mkdirAll(sm.persistDir, 0700)
	if err != nil {
		return nil, err
	}

	// Initialize the logger. Logging should be initialized ASAP, because the
	// rest of the initialization makes use of the logger.
	sm.log, err = dependencies.newLogger(filepath.Join(sm.persistDir, logFile))
	if err != nil {
		return nil, err
	}

	// Open the database containing the host's storage obligation metadata.
	sm.db, err = dependencies.openDatabase(dbMetadata, filepath.Join(sm.persistDir, dbFilename))
	if err != nil {
		// An error will be returned if the database has the wrong version, but
		// as of writing there was only one version of the database and all
		// other databases would be incompatible.
		_ = sm.log.Close()
		return nil, err
	}
	// After opening the database, it must be initalized. Most commonly,
	// nothing happens. But for new databases, a set of buckets must be
	// created. Intialization is also a good time to run sanity checks.
	err = sm.initDB()
	if err != nil {
		_ = sm.log.Close()
		_ = sm.db.Close()
		return nil, err
	}

	// Load the prior persistance structures.
	err = sm.load()
	if err != nil {
		_ = sm.log.Close()
		_ = sm.db.Close()
		return nil, err
	}
	return sm, nil
}

// New returns an initialized StorageManager.
func New(persistDir string) (*StorageManager, error) {
	return newStorageManager(productionDependencies{}, persistDir)
}
