package contractmanager

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
)

type (
	// storageFolderRemoval indicates a storage folder that has been removed
	// from the WAL.
	storageFolderRemoval struct {
		Index uint16
		Path  string
	}
)

var (
	// errPartialRelocation is returned during an operation attempting to clear
	// out the sectors in a storage folder if errors prevented one or more of
	// the sectors from being properly migrated to a new storage folder.
	errPartialRelocation = errors.New("unable to migrate all sectors")
)

// TODO: Sector operations must return an error if they are requested on a
// storage folder that is currently undergoing a modification. This only really
// applies to Remove and Delete.

// TODO: Definitely test performing all of the operations concurrently in the
// host while the host has a large number of sectors and a large amount of free
// space. Pair this with thorough checks to make sure the data being written
// and read is fully accurate.

// Adjusting a storage folder:
//	1. Writelock the storage folder.
//	2. Recycle code from AddSector and DeleteSector to migrate sectors one-at-a-time.
//	3. Commit a storage folder adjustment to the WAL.

// commitRemoveStorageFolder will finalize a storage folder removal from the
// contract manager.
func (wal *writeAheadLog) commitRemoveStorageFolder(sfr storageFolderRemoval) {
	// Close any open file handles.
	sf, exists := wal.cm.storageFolders[sfr.Index]
	if exists {
		sf.metadataFile.Close()
		sf.sectorFile.Close()
	}

	// Delete the files.
	os.Remove(filepath.Join(sfr.Path, metadataFile))
	os.Remove(filepath.Join(sfr.Path, sectorFile))
	delete(wal.cm.storageFolders, sfr.Index)
}

// managedMoveSector will move a sector from its current storage folder to
// another.
func (wal *writeAheadLog) managedMoveSector(id sectorID) error {
	wal.managedLockSector(id)
	defer wal.managedUnlockSector(id)

	// Find the sector to be moved.
	wal.mu.Lock()
	oldLocation, exists1 := wal.cm.sectorLocations[id]
	oldFolder, exists2 := wal.cm.storageFolders[oldLocation.storageFolder]
	wal.mu.Unlock()
	if !exists1 || !exists2 {
		return errors.New("unable to find sector that is targeted for move")
	}

	// Read the sector data from disk so that it can be added correctly to a
	// new storage folder.
	sectorData, err := readSector(oldFolder.sectorFile, oldLocation.index)
	if err != nil {
		atomic.AddUint64(&oldFolder.atomicFailedReads, 1)
		return build.ExtendErr("unable to read sector selected for migration", err)
	}
	atomic.AddUint64(&oldFolder.atomicSuccessfulReads, 1)

	// Create the sector update that will remove the old sector.
	oldSU := sectorUpdate{
		Count:  0,
		ID:     id,
		Folder: oldLocation.storageFolder,
		Index:  oldLocation.index,
	}

	// Place the sector into its new folder and add the atomic move to the WAL.
	wal.mu.Lock()
	storageFolders := wal.cm.storageFolderSlice()
	wal.mu.Unlock()
	var syncChan chan struct{}
	for len(storageFolders) >= 1 {
		var storageFolderIndex int
		err := func() error {
			// NOTE: Convention is broken when working with WAL lock here, due
			// to the complexity required with managing both the WAL lock and
			// the storage folder lock. Pay close attention when reviewing and
			// modifying.

			// Grab a vacant storage folder.
			wal.mu.Lock()
			var sf *storageFolder
			sf, storageFolderIndex = vacancyStorageFolder(storageFolders)
			if sf == nil {
				// None of the storage folders have enough room to house the
				// sector.
				wal.mu.Unlock()
				return errInsufficientStorageForSector
			}
			defer sf.mu.RUnlock()

			// Grab a sector from the storage folder. WAL lock cannot be
			// released between grabbing the storage folder and grabbing a
			// sector lest another thread request the final available sector in
			// the storage folder.
			sectorIndex, err := randFreeSector(sf.usage)
			if err != nil {
				wal.mu.Unlock()
				wal.cm.log.Critical("a storage folder with full usage was returned from emptiestStorageFolder")
				return err
			}
			// Set the usage, but mark it as uncommitted.
			sf.setUsage(sectorIndex)
			sf.availableSectors[id] = sectorIndex
			wal.mu.Unlock()

			// NOTE: The usage has been set, in the event of failure the usage
			// must be cleared.

			// Try writing the new sector to disk.
			err = writeSector(sf.sectorFile, sectorIndex, sectorData)
			if err != nil {
				wal.cm.log.Printf("ERROR: Unable to write sector for folder %v: %v\n", sf.path, err)
				atomic.AddUint64(&sf.atomicFailedWrites, 1)
				wal.mu.Lock()
				sf.clearUsage(sectorIndex)
				delete(sf.availableSectors, id)
				wal.mu.Unlock()
				return errDiskTrouble
			}

			// Try writing the sector metadata to disk.
			su := sectorUpdate{
				Count:  oldLocation.count,
				ID:     id,
				Folder: sf.index,
				Index:  sectorIndex,
			}
			err = wal.writeSectorMetadata(sf, su)
			if err != nil {
				wal.cm.log.Printf("ERROR: Unable to write sector metadata for folder %v: %v\n", sf.path, err)
				atomic.AddUint64(&sf.atomicFailedWrites, 1)
				wal.mu.Lock()
				sf.clearUsage(sectorIndex)
				delete(sf.availableSectors, id)
				wal.mu.Unlock()
				return errDiskTrouble
			}

			// Sector added successfully, update the WAL and the state.
			sl := sectorLocation{
				index:         sectorIndex,
				storageFolder: sf.index,
				count:         oldLocation.count,
			}
			wal.mu.Lock()
			wal.appendChange(stateChange{
				SectorUpdates: []sectorUpdate{oldSU, su},
			})
			oldFolder.clearUsage(oldLocation.index)
			delete(wal.cm.sectorLocations, oldSU.ID)
			delete(sf.availableSectors, id)
			wal.cm.sectorLocations[id] = sl
			syncChan = wal.syncChan
			wal.mu.Unlock()
			return nil
		}()
		if err != nil {
			// Try the next storage folder.
			storageFolders = append(storageFolders[:storageFolderIndex], storageFolders[storageFolderIndex+1:]...)
			continue
		}
		// Sector added successfully, break.
		break
	}
	if len(storageFolders) < 1 {
		return errInsufficientStorageForSector
	}
	return nil
}

// managedEmptyStorageFolder will empty out the storage folder with the
// provided index starting with the 'startingPoint'th sector all the way to the
// end of the storage folder, allowing the storage folder to be safely
// truncated. If 'force' is set to true, the function will not give up when
// there is no more space available, instead choosing to lose data.
//
// This function assumes that the storage folder has already been made
// invisible to AddSector, and that this is the only thread that will be
// interacting with the storage folder.
func (wal *writeAheadLog) managedEmptyStorageFolder(sfIndex uint16, startingPoint uint32) (uint64, error) {
	// Grab the storage folder in question.
	wal.mu.Lock()
	sf, exists := wal.cm.storageFolders[sfIndex]
	wal.mu.Unlock()
	if !exists {
		return 0, errBadStorageFolderIndex
	}

	// Read the sector lookup bytes into memory; we'll need them to figure out
	// what sectors are in which locations.
	sectorLookupBytes, err := readFullMetadata(sf.metadataFile, len(sf.usage)*storageFolderGranularity)
	if err != nil {
		atomic.AddUint64(&sf.atomicFailedReads, 1)
		return 0, build.ExtendErr("unable to read sector metadata", err)
	}
	atomic.AddUint64(&sf.atomicSuccessfulReads, 1)

	// Iterate through all of the sectors and perform the move operation on
	// them.
	var errCount uint64
	var wg sync.WaitGroup
	var readHead int
	for _, usage := range sf.usage[startingPoint/storageFolderGranularity:] {
		// The usage is a bitfield indicating where sectors exist. Iterate
		// through each bit to check for a sector.
		usageMask := uint64(1)
		for j := 0; j < storageFolderGranularity; j++ {
			// Perform a move operation if a sector exists in this location.
			if usage&usageMask == usageMask {
				// Fetch the id of the sector in this location.
				var id sectorID
				copy(id[:], sectorLookupBytes[readHead:readHead+12])
				// Reference the sector locations map to get the most
				// up-to-date status for the sector.
				wal.mu.Lock()
				_, exists := wal.cm.sectorLocations[id]
				wal.mu.Unlock()
				if !exists {
					// The sector has been deleted, but the usage has not been
					// updated yet. Safe to ignore.
					continue
				}

				// Queue the sector move. The queue will handle multithreading
				// and throughput optimization.
				wg.Add(1)
				wal.queueSectorMove(&wg, id, &errCount)
			}
			readHead += sectorMetadataDiskSize
			usageMask = usageMask << 1
		}
	}
	wg.Wait()

	// Return errPartialRelocation if not every sector was migrated out
	// successfully.
	if errCount > 0 {
		return errCount, errPartialRelocation
	}
	return 0, nil
}

// queueSectorMove will block until a thread is available to handle the move
// operation, and then will pass off the operation to that thread.
// queueSectorMove will also dynamically scale the threadpool size?
func (wal *writeAheadLog) queueSectorMove(wg *sync.WaitGroup, id sectorID, errCount *uint64) {
	// TODO: Implement a smarter thread pool. Millions of goroutines for a
	// large resize is totally unacceptable.
	go func() {
		defer wg.Done()
		err := wal.managedMoveSector(id)
		if err != nil {
			atomic.AddUint64(errCount, 1)
		}
	}()
}

// RemoveStorageFolder will delete a storage folder from the contract manager,
// moving all of the sectors in the storage folder to new storage folders.
func (cm *ContractManager) RemoveStorageFolder(index uint16, force bool) error {
	cm.tg.Add()
	defer cm.tg.Done()

	// Retrieve the specified storage folder.
	cm.wal.mu.Lock()
	sf, exists := cm.storageFolders[index]
	if !exists {
		cm.wal.mu.Unlock()
		return errStorageFolderNotFound
	}
	cm.wal.mu.Unlock()

	// Lock the storage folder for the duration of the operation.
	sf.mu.Lock()
	defer sf.mu.Unlock()

	// Clear out the sectors in the storage folder.
	_, err := cm.wal.managedEmptyStorageFolder(index, 0)
	if err != nil && !force {
		return err
	}

	// Wait for a synchronize to confirm that all of the moves have succeeded
	// in full.
	cm.wal.mu.Lock()
	syncChan := cm.wal.syncChan
	cm.wal.mu.Unlock()
	<-syncChan

	// Submit a storage folder removal to the WAL and wait until the update is
	// synced.
	cm.wal.mu.Lock()
	cm.wal.appendChange(stateChange{
		StorageFolderRemovals: []storageFolderRemoval{{
			Index: index,
			Path:  sf.path,
		}},
	})
	delete(cm.storageFolders, index)

	// Wait until the removal action has been synchronized.
	syncChan = cm.wal.syncChan
	cm.wal.mu.Unlock()
	<-syncChan

	// Remove the storage folder. Close all handles, and remove the files from
	// disk.
	//
	// TODO: In the rare event that this doesn't happen until after the deleted
	// cm.storageFolders settings update has synchronized, clutter may be left
	// on disk.
	//
	// TODO: Handle these by doing them during the WAL commit.
	err = sf.metadataFile.Close()
	if err != nil {
		cm.log.Printf("Error: unable to close metadata file as storage folder %v is removed\n", sf.path)
	}
	err = sf.sectorFile.Close()
	if err != nil {
		cm.log.Printf("Error: unable to close sector file as storage folder %v is removed\n", sf.path)
	}
	err = os.Remove(filepath.Join(sf.path, metadataFile))
	if err != nil {
		cm.log.Printf("Error: unable to remove metadata file as storage folder %v is removed\n", sf.path)
	}
	err = os.Remove(filepath.Join(sf.path, sectorFile))
	if err != nil {
		cm.log.Printf("Error: unable to reomve sector file as storage folder %v is removed\n", sf.path)
	}
	return nil
}

// shrinkStoragefolder will truncate a storage folder, moving all of the
// sectors in the truncated space to new storage folders.
func (wal *writeAheadLog) shrinkStorageFolder(index uint16, newSectorCount uint32, force bool) error {
	// Retrieve the specified storage folder.
	wal.mu.Lock()
	sf, exists := wal.cm.storageFolders[index]
	if !exists {
		wal.mu.Unlock()
		return errStorageFolderNotFound
	}
	wal.mu.Unlock()

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

	// Truncate the storage folder.
	err = sf.metadataFile.Truncate(int64(newSectorCount * sectorMetadataDiskSize))
	if err != nil {
		wal.cm.log.Printf("Error: unable to truncate metadata file as storage folder %v is resized\n", sf.path)
	}
	err = sf.sectorFile.Truncate(int64(modules.SectorSize * uint64(newSectorCount)))
	if err != nil {
		wal.cm.log.Printf("Error: unable to truncate sector file as storage folder %v is resized\n", sf.path)
	}
	return nil
}
