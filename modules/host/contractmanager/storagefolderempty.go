package contractmanager

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/NebulousLabs/Sia/build"
)

var (
	// ErrPartialRelocation is returned during an operation attempting to clear
	// out the sectors in a storage folder if errors prevented one or more of
	// the sectors from being properly migrated to a new storage folder.
	ErrPartialRelocation = errors.New("unable to migrate all sectors")
)

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
	if !exists1 || !exists2 || atomic.LoadUint64(&oldFolder.atomicUnavailable) == 1 {
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
	storageFolders := wal.cm.availableStorageFolders()
	wal.mu.Unlock()
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
			wal.mu.Unlock()
			return nil
		}()
		if err == errInsufficientStorageForSector {
			return err
		} else if err != nil {
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
	if !exists || atomic.LoadUint64(&sf.atomicUnavailable) == 1 {
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

	// Before iterating through the sectors and moving them, set up a thread
	// pool that can parallelize the transfers without spinning up 250,000
	// goroutines per TB.
	var errCount uint64
	var wg sync.WaitGroup
	workers := 250
	workChan := make(chan sectorID)
	doneChan := make(chan struct{})
	for i := 0; i < workers; i++ {
		go func() {
			for {
				select {
				case id := <-workChan:
					err := wal.managedMoveSector(id)
					if err != nil {
						atomic.AddUint64(&errCount, 1)
						wal.cm.log.Println("Unable to write sector:", err)
					}
					wg.Done()
				case <-doneChan:
					return
				}
			}
		}()
	}

	// Iterate through all of the sectors and perform the move operation on
	// them.
	readHead := startingPoint * sectorMetadataDiskSize
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

				// Queue the sector move.
				wg.Add(1)
				workChan <- id
			}
			readHead += sectorMetadataDiskSize
			usageMask = usageMask << 1
		}
	}
	wg.Wait()
	close(doneChan)

	// Return errPartialRelocation if not every sector was migrated out
	// successfully.
	if errCount > 0 {
		return errCount, ErrPartialRelocation
	}
	return 0, nil
}
