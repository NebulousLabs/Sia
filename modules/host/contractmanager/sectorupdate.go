package contractmanager

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

// commitUpdateSector will commit a sector update to the contract manager,
// writing in metadata and usage info if the sector still exists, and deleting
// the usage info if the sector does not exist. The update is idempotent.
func (wal *writeAheadLog) commitUpdateSector(su sectorUpdate) {
	sf, exists := wal.cm.storageFolders[su.Folder]
	if !exists || atomic.LoadUint64(&sf.atomicUnavailable) == 1 {
		wal.cm.log.Printf("ERROR: unable to locate storage folder for a committed sector update.")
		return
	}

	// If the sector is being cleaned from disk, unset the usage flag.
	if su.Count == 0 {
		sf.clearUsage(su.Index)
		return
	}

	// Set the usage flag and update the on-disk metadata. Abort if the
	// metadata write fails.
	err := wal.writeSectorMetadata(sf, su)
	if err != nil {
		wal.cm.log.Printf("ERROR: unable to write sector metadata for %v: %v\n", sf.path, err)
		return
	}
	sf.setUsage(su.Index)
}

// managedAddPhysicalSector is a WAL operation to add a physical sector to the
// contract manager.
func (wal *writeAheadLog) managedAddPhysicalSector(id sectorID, data []byte, count uint16) error {
	// Sanity check - data should have modules.SectorSize bytes.
	if uint64(len(data)) != modules.SectorSize {
		wal.cm.log.Critical("sector has the wrong size", modules.SectorSize, len(data))
		return errors.New("malformed sector")
	}

	// Find a committed storage folder that has enough space to receive
	// this sector. Keep trying new storage folders if some return
	// errors during disk operations.
	wal.mu.Lock()
	storageFolders := wal.cm.availableStorageFolders()
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
			err = writeSector(sf.sectorFile, sectorIndex, data)
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
				Count:  count,
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
				count:         count,
			}
			wal.mu.Lock()
			wal.appendChange(stateChange{
				SectorUpdates: []sectorUpdate{su},
			})
			delete(wal.cm.storageFolders[su.Folder].availableSectors, id)
			wal.cm.sectorLocations[id] = sl
			syncChan = wal.syncChan
			wal.mu.Unlock()
			return nil
		}()
		if err != nil {
			// End the loop if no storage folder proved suitable.
			if storageFolderIndex == -1 {
				storageFolders = nil
				break
			}

			// Remove the storage folder that failed and try the next one.
			storageFolders = append(storageFolders[:storageFolderIndex], storageFolders[storageFolderIndex+1:]...)
			continue
		}
		// Sector added successfully, break.
		break
	}
	if len(storageFolders) < 1 {
		return errInsufficientStorageForSector
	}

	// Wait for the synchronize.
	// sectors.
	<-syncChan
	return nil
}

// managedAddVirtualSector will add a virtual sector to the contract manager.
func (wal *writeAheadLog) managedAddVirtualSector(id sectorID, location sectorLocation) error {
	// Update the location count.
	if location.count == 65535 {
		return errMaxVirtualSectors
	}
	location.count++

	// Prepare the sector update.
	su := sectorUpdate{
		Count:  location.count,
		ID:     id,
		Folder: location.storageFolder,
		Index:  location.index,
	}

	// Append the sector update to the WAL.
	wal.mu.Lock()
	sf, exists := wal.cm.storageFolders[su.Folder]
	if !exists || atomic.LoadUint64(&sf.atomicUnavailable) == 1 {
		// Need to check that the storage folder exists before syncing the
		// commit that increases the virtual sector count.
		wal.mu.Unlock()
		return errStorageFolderNotFound
	}
	wal.appendChange(stateChange{
		SectorUpdates: []sectorUpdate{su},
	})
	wal.cm.sectorLocations[id] = location
	syncChan := wal.syncChan
	wal.mu.Unlock()
	<-syncChan

	// Update the metadata on disk. Metadata is updated on disk after the sync
	// so that there is no risk of obliterating the previous count in the event
	// that the change is not fully committed during unclean shutdown.
	err := wal.writeSectorMetadata(sf, su)
	if err != nil {
		// Revert the sector update in the WAL to reflect the fact that adding
		// the sector has failed.
		su.Count--
		location.count--
		wal.mu.Lock()
		wal.appendChange(stateChange{
			SectorUpdates: []sectorUpdate{su},
		})
		wal.cm.sectorLocations[id] = location
		wal.mu.Unlock()
		<-syncChan
		return build.ExtendErr("unable to write sector metadata during addSector call", err)
	}
	return nil
}

// managedDeleteSector will delete a sector (physical) from the contract manager.
func (wal *writeAheadLog) managedDeleteSector(id sectorID) error {
	// Write the sector delete to the WAL.
	var location sectorLocation
	var syncChan chan struct{}
	var sf *storageFolder
	err := func() error {
		wal.mu.Lock()
		defer wal.mu.Unlock()

		// Fetch the metadata related to the sector.
		var exists bool
		location, exists = wal.cm.sectorLocations[id]
		if !exists {
			return ErrSectorNotFound
		}
		sf, exists = wal.cm.storageFolders[location.storageFolder]
		if !exists || atomic.LoadUint64(&sf.atomicUnavailable) == 1 {
			wal.cm.log.Critical("deleting a sector from a storage folder that does not exist?")
			return errStorageFolderNotFound
		}

		// Inform the WAL of the sector update.
		wal.appendChange(stateChange{
			SectorUpdates: []sectorUpdate{{
				Count:  0,
				ID:     id,
				Folder: location.storageFolder,
				Index:  location.index,
			}},
		})

		// Delete the sector and mark the usage as available.
		delete(wal.cm.sectorLocations, id)
		sf.availableSectors[id] = location.index

		// Block until the change has been committed.
		syncChan = wal.syncChan
		return nil
	}()
	if err != nil {
		return err
	}
	<-syncChan

	// Only update the usage after the sector delete has been committed to disk
	// fully.
	wal.mu.Lock()
	delete(sf.availableSectors, id)
	sf.clearUsage(location.index)
	wal.mu.Unlock()
	return nil
}

// managedRemoveSector will remove a sector (virtual or physical) from the
// contract manager.
func (wal *writeAheadLog) managedRemoveSector(id sectorID) error {
	// Inform the WAL of the removed sector.
	var location sectorLocation
	var su sectorUpdate
	var sf *storageFolder
	var syncChan chan struct{}
	err := func() error {
		wal.mu.Lock()
		defer wal.mu.Unlock()

		// Grab the number of virtual sectors that have been committed with
		// this root.
		var exists bool
		location, exists = wal.cm.sectorLocations[id]
		if !exists {
			return ErrSectorNotFound
		}
		sf, exists = wal.cm.storageFolders[location.storageFolder]
		if !exists || atomic.LoadUint64(&sf.atomicUnavailable) == 1 {
			wal.cm.log.Critical("deleting a sector from a storage folder that does not exist?")
			return errStorageFolderNotFound
		}

		// Inform the WAL of the sector update.
		location.count--
		su = sectorUpdate{
			Count:  location.count,
			ID:     id,
			Folder: location.storageFolder,
			Index:  location.index,
		}
		wal.appendChange(stateChange{
			SectorUpdates: []sectorUpdate{su},
		})

		// Update the in-memeory representation of the sector.
		if location.count == 0 {
			// Delete the sector and mark it as available.
			delete(wal.cm.sectorLocations, id)
			sf.availableSectors[id] = location.index
		} else {
			// Reduce the sector usage.
			wal.cm.sectorLocations[id] = location
		}
		syncChan = wal.syncChan
		return nil
	}()
	if err != nil {
		return err
	}
	// synchronize before updating the metadata or clearing the usage.
	<-syncChan

	// Update the metadata, and the usage.
	if location.count != 0 {
		err = wal.writeSectorMetadata(sf, su)
		if err != nil {
			// Revert the previous change.
			wal.mu.Lock()
			su.Count++
			location.count++
			wal.appendChange(stateChange{
				SectorUpdates: []sectorUpdate{su},
			})
			wal.cm.sectorLocations[id] = location
			wal.mu.Unlock()
			return build.ExtendErr("failed to write sector metadata", err)
		}
	}

	// Only update the usage after the sector removal has been committed to
	// disk entirely. The usage is not updated until after the commit has
	// completed to prevent the actual sector data from being overwritten in
	// the event of unclean shutdown.
	if location.count == 0 {
		wal.mu.Lock()
		sf.clearUsage(location.index)
		delete(sf.availableSectors, id)
		wal.mu.Unlock()
	}
	return nil
}

// writeSectorMetadata will take a sector update and write the related metadata
// to disk.
func (wal *writeAheadLog) writeSectorMetadata(sf *storageFolder, su sectorUpdate) error {
	err := writeSectorMetadata(sf.metadataFile, su.Index, su.ID, su.Count)
	if err != nil {
		wal.cm.log.Printf("ERROR: unable to write sector metadata to folder %v when adding sector: %v\n", su.Folder, err)
		atomic.AddUint64(&sf.atomicFailedWrites, 1)
		return err
	}
	atomic.AddUint64(&sf.atomicSuccessfulWrites, 1)
	return nil
}

// AddSector will add a sector to the contract manager.
func (cm *ContractManager) AddSector(root crypto.Hash, sectorData []byte) error {
	// Prevent shutdown until this function completes.
	err := cm.tg.Add()
	if err != nil {
		return err
	}
	defer cm.tg.Done()

	// Hold a sector lock throughout the duration of the function, but release
	// before syncing.
	id := cm.managedSectorID(root)
	cm.wal.managedLockSector(id)
	defer cm.wal.managedUnlockSector(id)

	// Determine whether the sector is virtual or physical.
	cm.wal.mu.Lock()
	location, exists := cm.sectorLocations[id]
	cm.wal.mu.Unlock()
	if exists {
		err = cm.wal.managedAddVirtualSector(id, location)
	} else {
		err = cm.wal.managedAddPhysicalSector(id, sectorData, 1)
	}
	if err != nil {
		cm.log.Println("ERROR: Unable to add sector:", err)
		return err
	}
	return nil
}

// AddSectorBatch is a non-ACID call to add a bunch of sectors at once.
// Necessary for compatibility with old renters.
//
// TODO: Make ACID, and definitely improve the performance as well.
func (cm *ContractManager) AddSectorBatch(sectorRoots []crypto.Hash) error {
	// Prevent shutdown until this function completes.
	err := cm.tg.Add()
	if err != nil {
		return err
	}
	defer cm.tg.Done()

	// Add each sector in a separate goroutine.
	var wg sync.WaitGroup
	for _, root := range sectorRoots {
		wg.Add(1)
		go func(root crypto.Hash) {
			defer wg.Done()

			// Hold a sector lock throughout the duration of the function, but release
			// before syncing.
			id := cm.managedSectorID(root)
			cm.wal.managedLockSector(id)
			defer cm.wal.managedUnlockSector(id)

			// Add the sector as virtual.
			cm.wal.mu.Lock()
			location, exists := cm.sectorLocations[id]
			cm.wal.mu.Unlock()
			if exists {
				cm.wal.managedAddVirtualSector(id, location)
			}
		}(root)
	}
	wg.Wait()
	return nil
}

// DeleteSector will delete a sector from the contract manager. If multiple
// copies of the sector exist, all of them will be removed. This should only be
// used to remove offensive data, as it will cause corruption in the contract
// manager. This corruption puts the contract manager at risk of failing
// storage proofs. If the amount of data removed is small, the risk is small.
// This operation will not destabilize the contract manager.
func (cm *ContractManager) DeleteSector(root crypto.Hash) error {
	cm.tg.Add()
	defer cm.tg.Done()
	id := cm.managedSectorID(root)
	cm.wal.managedLockSector(id)
	defer cm.wal.managedUnlockSector(id)

	return cm.wal.managedDeleteSector(id)
}

// RemoveSector will remove a sector from the contract manager. If multiple
// copies of the sector exist, only one will be removed.
func (cm *ContractManager) RemoveSector(root crypto.Hash) error {
	cm.tg.Add()
	defer cm.tg.Done()
	id := cm.managedSectorID(root)
	cm.wal.managedLockSector(id)
	defer cm.wal.managedUnlockSector(id)

	return cm.wal.managedRemoveSector(id)
}

// RemoveSectorBatch is a non-ACID call to remove a bunch of sectors at once.
// Necessary for compatibility with old renters.
//
// TODO: Make ACID, and definitely improve the performance as well.
func (cm *ContractManager) RemoveSectorBatch(sectorRoots []crypto.Hash) error {
	// Prevent shutdown until this function completes.
	err := cm.tg.Add()
	if err != nil {
		return err
	}
	defer cm.tg.Done()

	// Add each sector in a separate goroutine.
	var wg sync.WaitGroup
	for _, root := range sectorRoots {
		wg.Add(1)
		go func(root crypto.Hash) {
			id := cm.managedSectorID(root)
			cm.wal.managedLockSector(id)
			cm.wal.managedRemoveSector(id) // Error is ignored.
			cm.wal.managedUnlockSector(id)
			wg.Done()
		}(root)
	}
	wg.Wait()
	return nil
}
