package contractmanager

// TODO: The locking on the whole thing needs to be code reviewed - can't act
// on a sector location without having a lock on it the whole time.

import (
	"errors"
	"sync/atomic"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

// commitUpdateSector will commit a sector update to the contract manager,
// writing in metadata and usage info if the sector still exists, and deleting
// the usage info if the sector does not exist. The update is idempotent.
func (wal *writeAheadLog) commitUpdateSector(su sectorUpdate) {
	sf := wal.cm.storageFolders[su.Folder]

	// If the sector is being cleaned from disk, unset the usage flag.
	if su.Count == 0 {
		sf.clearUsage(su.Index)
		return
	}

	// Set the usage flag and update the on-disk metadata.
	sf.setUsage(su.Index)
	wal.writeSectorMetadata(sf, su)
}

// managedAddPhysicalSector is a WAL operation to add a physical sector to the
// contract manager.
func (wal *writeAheadLog) managedAddPhysicalSector(id sectorID, data []byte, count uint16) (sectorLocation, error) {
	// Sanity check - data should have modules.SectorSize bytes.
	if uint64(len(data)) != modules.SectorSize {
		wal.cm.log.Critical("sector has the wrong size", modules.SectorSize, len(data))
		return sectorLocation{}, errors.New("malformed sector")
	}

	// Find a committed storage folder that has enough space to receive
	// this sector. Keep trying new storage folders if some return
	// errors during disk operations.
	wal.mu.Lock()
	storageFolders := wal.cm.storageFolderSlice()
	wal.mu.Unlock()
	var sectorIndex uint32
	var sf *storageFolder
	var storageFolderIndex int
	for {
		wal.mu.Lock()
		sf, storageFolderIndex = vacancyStorageFolder(storageFolders)
		if sf == nil {
			// None of the storage folders have enough room to house the
			// sector.
			wal.mu.Unlock()
			return sectorLocation{}, errInsufficientStorageForSector
		}
		var err error
		sectorIndex, err = randFreeSector(sf.usage)
		if err != nil {
			wal.mu.Unlock()
			wal.cm.log.Critical("a storage folder with full usage was returned from emptiestStorageFolder")
			return sectorLocation{}, err
		}
		sf.setUsage(sectorIndex)
		// Mark this usage as uncommitted.
		sf.availableSectors[id] = sectorIndex
		wal.mu.Unlock()
		// Release the RLock that is grabbed by the vacancyStorageFolder once
		// we have finished adding the sector.
		defer sf.mu.RUnlock()
		err = func() error {
			// Write the new sector to disk. In the event of an error, clear
			// the usage.
			err := writeSector(sf.sectorFile, sectorIndex, data)
			if err != nil {
				wal.cm.log.Printf("ERROR: Unable to write sector for folder %v: %v\n", sf.path, err)
				atomic.AddUint64(&sf.atomicFailedWrites, 1)
				wal.mu.Lock()
				sf.clearUsage(sectorIndex)
				delete(sf.availableSectors, id)
				wal.mu.Unlock()
				return errDiskTrouble
			}
			su := sectorUpdate{
				Count:  count,
				ID:     id,
				Folder: sf.index,
				Index:  sectorIndex,
			}
			err = wal.writeSectorMetadata(sf, su)
			if err != nil {
				delete(wal.cm.storageFolders[su.Folder].availableSectors, su.ID)
				return build.ExtendErr("unable to write sector metadata during addSector call", err)
			}
			return nil
		}()
		if err == nil {
			// Sector added to a storage folder successfully.
			break
		}
		// Sector not added to storage folder successfully, remove this
		// stoage folder from the list of storage folders, and try the
		// next one.
		storageFolders = append(storageFolders[:storageFolderIndex], storageFolders[storageFolderIndex+1:]...)
	}

	// Update the state to reflect the new sector.
	sl := sectorLocation{
		index:         sectorIndex,
		storageFolder: sf.index,
		count:         count,
	}
	wal.mu.Lock()
	wal.cm.sectorLocations[id] = sl
	wal.mu.Unlock()
	return sl, nil
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

		// Grab the number of virtual sectors that have been committed with
		// this root.
		var exists bool
		location, exists = wal.cm.sectorLocations[id]
		if !exists {
			wal.mu.Unlock()
			return errSectorNotFound
		}
		// Delete the sector from the sector locations map.
		delete(wal.cm.sectorLocations, id)

		// Add this sector to the list of inactive-but-techincally-available
		// usage bits.
		sf, exists = wal.cm.storageFolders[location.storageFolder]
		if !exists {
			wal.cm.log.Critical("deleting a sector from a storage folder that does not exist?")
			return errStorageFolderNotFound
		}
		sf.availableSectors[id] = location.index

		// Inform the WAL of the sector update.
		err := wal.appendChange(stateChange{
			SectorUpdates: []sectorUpdate{{
				Count:  0,
				ID:     id,
				Folder: location.storageFolder,
				Index:  location.index,
			}},
		})
		if err != nil {
			return build.ExtendErr("failed to add a state change", err)
		}

		// Block until the change has been committed.
		syncChan = wal.syncChan
		return nil
	}()
	if err != nil {
		return build.ExtendErr("failed to write to WAL", err)
	}
	<-syncChan

	// Only update the usage after the sector delete has been committed to disk
	// fully.
	wal.mu.Lock()
	defer wal.mu.Unlock()
	sf, exists := wal.cm.storageFolders[location.storageFolder]
	if !exists {
		wal.cm.log.Critical("storage folder housing an existing sector does not exist")
		return nil
	}
	delete(sf.availableSectors, id)
	sf.clearUsage(location.index)
	return nil
}

// managedRemoveSector will remove a sector (virtual or physical) from the
// contract manager.
//
// TODO: Need to mark the sectors in the available sectors map.
func (wal *writeAheadLog) managedRemoveSector(id sectorID) error {
	// Inform the WAL of the removed sector.
	var location sectorLocation
	var su sectorUpdate
	var sf *storageFolder
	err := func() error {
		wal.mu.Lock()
		defer wal.mu.Unlock()

		// Grab the number of virtual sectors that have been committed with
		// this root.
		var exists bool
		location, exists = wal.cm.sectorLocations[id]
		if !exists {
			return errSectorNotFound
		}
		location.count--

		// Inform the WAL of the sector update.
		su = sectorUpdate{
			Count:  location.count,
			ID:     id,
			Folder: location.storageFolder,
			Index:  location.index,
		}
		err := wal.appendChange(stateChange{
			SectorUpdates: []sectorUpdate{su},
		})
		if err != nil {
			return build.ExtendErr("failed to add a state change", err)
		}

		// Update the in memory representation of the sector (except the
		// usage), and write the new metadata to disk if needed.
		if location.count != 0 {
			wal.cm.sectorLocations[id] = location
			sf = wal.cm.storageFolders[location.storageFolder]
		} else {
			delete(wal.cm.sectorLocations, id)
		}
		return nil
	}()
	if err != nil {
		return err
	}

	if location.count != 0 {
		err = wal.writeSectorMetadata(sf, su)
		if err != nil {
			return build.ExtendErr("failed to write sector metadata", err)
		}
	}
	wal.mu.Lock()
	syncChan := wal.syncChan
	wal.mu.Unlock()
	<-syncChan

	// Only update the usage after the sector removal has been committed to
	// disk entirely. The usage is not updated until after the commit has
	// completed to prevent the actual sector data from being overwritten in
	// the event of unclean shutdown.
	if location.count == 0 {
		wal.mu.Lock()
		sf, exists := wal.cm.storageFolders[location.storageFolder]
		if !exists {
			wal.cm.log.Critical("storage folder housing an existing sector does not exist")
			wal.mu.Unlock()
			return nil
		}
		sf.clearUsage(location.index)
		wal.mu.Unlock()
	}

	// The altered usage must be committed as well.
	wal.mu.Lock()
	syncChan = wal.syncChan
	wal.mu.Unlock()
	<-syncChan
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
	cm.tg.Add()
	defer cm.tg.Done()

	var syncChan chan struct{}
	var su sectorUpdate
	err := func() error {
		id := cm.managedSectorID(root)
		cm.wal.managedLockSector(id)
		defer cm.wal.managedUnlockSector(id)

		// It's okay to be loose with the locks here because the sectorLocations
		// value for this sector will not be modified - modifying the sector
		// locations value would require the sector lock.
		cm.wal.mu.Lock()
		location, exists := cm.sectorLocations[id]
		cm.wal.mu.Unlock()
		if exists {
			// Update the location count to indicate that a sector has been
			// added.
			if location.count == 65535 {
				// Max virtual sectors reached, do not make change.
				return errMaxVirtualSectors
			}
			location.count += 1

			// Write the sector metadata to disk.
			su = sectorUpdate{
				Count:  location.count,
				ID:     id,
				Folder: location.storageFolder,
				Index:  location.index,
			}
			cm.wal.mu.Lock()
			sf := cm.storageFolders[su.Folder]
			cm.wal.mu.Unlock()
			err := cm.wal.writeSectorMetadata(sf, su)
			if err != nil {
				cm.wal.mu.Lock()
				delete(cm.storageFolders[su.Folder].availableSectors, su.ID)
				cm.wal.mu.Unlock()
				return build.ExtendErr("unable to write sector metadata during addSector call", err)
			}
		} else {
			var err error
			location, err = cm.wal.managedAddPhysicalSector(id, sectorData, 1)
			if err != nil {
				cm.log.Debugln("unable to add sector:", err)
				return build.ExtendErr("unable to add sector", err)
			}
			su = sectorUpdate{
				Count:  location.count,
				ID:     id,
				Folder: location.storageFolder,
				Index:  location.index,
			}
		}

		// Update the WAL.
		cm.wal.mu.Lock()
		defer cm.wal.mu.Unlock()
		delete(cm.storageFolders[su.Folder].availableSectors, su.ID)
		err := cm.wal.appendChange(stateChange{
			SectorUpdates: []sectorUpdate{su},
		})
		if err != nil {
			return err
		}
		cm.sectorLocations[id] = location
		syncChan = cm.wal.syncChan
		return nil
	}()
	if err != nil {
		return err
	}

	// Return after the commitment has been synchronized.
	<-syncChan
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
