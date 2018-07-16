package contractmanager

import (
	"sync/atomic"

	"gitlab.com/NebulousLabs/Sia/modules"
)

type (
	// storageFolderReduction dictates a completed storage folder reduction to
	// the WAL.
	storageFolderReduction struct {
		Index          uint16
		NewSectorCount uint32
	}
)

// commitStorageFolderReduction commits a storage folder reduction to the state
// and filesystem.
func (wal *writeAheadLog) commitStorageFolderReduction(sfr storageFolderReduction) {
	sf, exists := wal.cm.storageFolders[sfr.Index]
	if !exists {
		wal.cm.log.Critical("ERROR: storage folder reduction established for a storage folder that does not exist")
		return
	}
	if atomic.LoadUint64(&sf.atomicUnavailable) == 1 {
		// Cannot complete the storage folder reduction - storage folder is not
		// available.
		return
	}

	// Shrink the sector usage, but only if the sector usage is not already
	// smaller.
	if uint32(len(sf.usage)) > sfr.NewSectorCount/storageFolderGranularity {
		// Unset the usage in all bits
		for i := sfr.NewSectorCount; i < uint32(len(sf.usage))*storageFolderGranularity; i++ {
			sf.clearUsage(i)
		}
		// Truncate the usage field.
		sf.usage = sf.usage[:sfr.NewSectorCount/storageFolderGranularity]
	}

	// Truncate the storage folder.
	err := sf.metadataFile.Truncate(int64(sfr.NewSectorCount * sectorMetadataDiskSize))
	if err != nil {
		wal.cm.log.Printf("Error: unable to truncate metadata file as storage folder %v is resized\n", sf.path)
	}
	err = sf.sectorFile.Truncate(int64(modules.SectorSize * uint64(sfr.NewSectorCount)))
	if err != nil {
		wal.cm.log.Printf("Error: unable to truncate sector file as storage folder %v is resized\n", sf.path)
	}
}

// shrinkStoragefolder will truncate a storage folder, moving all of the
// sectors in the truncated space to new storage folders.
func (wal *writeAheadLog) shrinkStorageFolder(index uint16, newSectorCount uint32, force bool) error {
	// Retrieve the specified storage folder.
	wal.mu.Lock()
	sf, exists := wal.cm.storageFolders[index]
	wal.mu.Unlock()
	if !exists {
		return errStorageFolderNotFound
	}
	if atomic.LoadUint64(&sf.atomicUnavailable) == 1 {
		// TODO: Better error.
		return errStorageFolderNotFound
	}

	// Lock the storage folder for the duration of the operation.
	sf.mu.Lock()
	defer sf.mu.Unlock()

	// Clear out the sectors in the storage folder.
	_, err := wal.managedEmptyStorageFolder(index, newSectorCount)
	if err != nil && !force {
		return err
	}

	// Wait for a synchronize to confirm that all of the moves have succeeded
	// in full.
	wal.mu.Lock()
	syncChan := wal.syncChan
	wal.mu.Unlock()
	<-syncChan

	// Allow unclean shutdown to be simulated by returning before the state
	// change gets committed.
	if wal.cm.dependencies.Disrupt("incompleteShrinkStorageFolder") {
		return nil
	}

	// Submit a storage folder truncation to the WAL and wait until the update
	// is synced.
	wal.mu.Lock()
	wal.appendChange(stateChange{
		StorageFolderReductions: []storageFolderReduction{{
			Index:          index,
			NewSectorCount: newSectorCount,
		}},
	})
	syncChan = wal.syncChan
	wal.mu.Unlock()

	// Wait until the shrink action has been synchronized.
	<-syncChan
	return nil
}
