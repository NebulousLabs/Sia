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
	wal.writeSectorMetadata(su)
}

// managedAddPhysicalSector is a WAL operation to add a physical sector to the
// contract manager.
func (wal *writeAheadLog) managedAddPhysicalSector(id sectorID, data []byte) error {
	// Sanity check - data should have modules.SectorSize bytes.
	if uint64(len(data)) != modules.SectorSize {
		wal.cm.log.Critical("sector has the wrong size", modules.SectorSize, len(data))
		return errors.New("malformed sector")
	}

	// Find a committed storage folder that has enough space to receive
	// this sector. Keep trying new storage folders if some return
	// errors during disk operations.
	storageFolders := wal.cm.storageFolderSlice()
	sf := new(storageFolder)
	var sectorIndex uint32
	var storageFolderIndex int
	for {
		wal.mu.Lock()
		sf, storageFolderIndex = emptiestStorageFolder(storageFolders)
		wal.mu.Unlock()
		if sf == nil {
			// None of the storage folders have enough room to house the
			// sector.
			return errInsufficientStorageForSector
		}
		err := func() error {
			// Find a location for the sector within the file using the Usage
			// field.
			var err error
			wal.mu.Lock()
			sectorIndex, err = randFreeSector(sf.Usage)
			if err != nil {
				wal.mu.Unlock()
				wal.cm.log.Critical("a storage folder with full usage was returned from emptiestStorageFolder")
				return err
			}
			sf.setUsage(sectorIndex)
			// Mark this usage as uncommitted.
			sf.queuedSectors[id] = sectorIndex
			wal.mu.Unlock()

			// Write the new sector to disk. In the event of an error, clear
			// the usage.
			err = writeSector(sf.sectorFile, sectorIndex, data)
			if err != nil {
				wal.cm.log.Printf("ERROR: Unable to write sector for folder %v: %v\n", sf.Index, err)
				atomic.AddUint64(&sf.atomicFailedWrites, 1)
				wal.mu.Lock()
				sf.clearUsage(sectorIndex)
				delete(sf.queuedSectors, id)
				wal.mu.Unlock()
				return errDiskTrouble
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
	wal.mu.Lock()
	wal.cm.sectorLocations[id] = sectorLocation{
		index:         sectorIndex,
		storageFolder: sf.Index,
		count:         1,
	}
	sf.sectors += 1
	wal.mu.Unlock()

	return wal.managedApplySectorUpdate(sectorUpdate{
		Count:  1,
		ID:     id,
		Folder: sf.Index,
		Index:  sectorIndex,
	})
}

// managedApplySectorUpdate will apply the provided sector update to the
// contract manager.
func (wal *writeAheadLog) managedApplySectorUpdate(su sectorUpdate) error {
	// Write the sector metadata to disk.
	err := wal.writeSectorMetadata(su)
	if err != nil {
		return build.ExtendErr("unable to write sector metadata during addSector call", err)
	}

	// Update the WAL, which requires appending a change, then grabbing a sync
	// channel, then updating the queuedSectors map.
	var syncChan chan struct{}
	err = func() error {
		wal.mu.Lock()
		defer wal.mu.Unlock()

		err = wal.appendChange(stateChange{
			SectorUpdates: []sectorUpdate{su},
		})
		syncChan = wal.syncChan
		// The sector usage which was previously only queued has now come into
		// full effect.
		delete(wal.cm.storageFolders[su.Folder].queuedSectors, su.ID)
		return err
	}()
	if err != nil {
		return build.ExtendErr("failed to add a state change", err)
	}

	// Return after the commitment has been synchronized.
	<-syncChan
	return nil
}

// managedDeleteSector will delete a sector (physical) from the contract manager.
func (wal *writeAheadLog) managedDeleteSector(id sectorID) error {
	// Create and fill out the sectorUpdate object.
	su := sectorUpdate{
		ID: id,
	}
	var syncChan chan struct{}
	err := func() error {
		wal.mu.Lock()
		defer wal.mu.Unlock()

		// Grab the number of virtual sectors that have been committed with
		// this root.
		location, exists := wal.cm.sectorLocations[id]
		if !exists {
			return errSectorNotFound
		}
		// Delete the sector from the sector locations map.
		delete(wal.cm.sectorLocations, id)

		// Fill out the sectorUpdate object so that it can be added to the
		// WAL.
		su.Count = 0 // This function is only being called if we want to set the count to zero.
		su.Folder = location.storageFolder
		su.Index = location.index

		// Inform the WAL of the sector update.
		err := wal.appendChange(stateChange{
			SectorUpdates: []sectorUpdate{su},
		})
		if err != nil {
			return build.ExtendErr("failed to add a state change", err)
		}

		// Grab the sync channel to know when the update has been durably
		// committed.
		syncChan = wal.syncChan
		return nil
	}()
	if err != nil {
		return build.ExtendErr("cannot delete sector:", err)
	}

	// Only return after the commitment has succeeded fully.
	<-syncChan

	// Only update the usage after the sector delete has been committed to disk
	// fully.
	//
	// The usage is not updated until after the commit has completed to prevent
	// the actual sector data from being overwritten in the event of unclean
	// shutdown.
	wal.mu.Lock()
	defer wal.mu.Unlock()
	usageElement := wal.cm.storageFolders[su.Folder].Usage[su.Index/storageFolderGranularity]
	bitIndex := su.Index % storageFolderGranularity
	usageElement = usageElement & (^(1 << bitIndex))
	wal.cm.storageFolders[su.Folder].Usage[su.Index/storageFolderGranularity] = usageElement
	wal.cm.storageFolders[su.Folder].sectors--
	return nil
}

// managedRemoveSector will remove a sector (virtual or physical) from the
// contract manager.
func (wal *writeAheadLog) managedRemoveSector(id sectorID) error {
	// Create and fill out the sectorUpdate object.
	su := sectorUpdate{
		ID: id,
	}
	var syncChan chan struct{}
	err := func() error {
		wal.mu.Lock()
		defer wal.mu.Unlock()

		// Check that the storage folder is not currently undergoing a move
		// operation.

		// Grab the number of virtual sectors that have been committed with
		// this root.
		location, exists := wal.cm.sectorLocations[id]
		if !exists {
			return errSectorNotFound
		}
		location.count--

		// Fill out the sectorUpdate object so that it can be added to the
		// WAL.
		su.Count = location.count
		su.Folder = location.storageFolder
		su.Index = location.index

		// Inform the WAL of the sector update.
		err := wal.appendChange(stateChange{
			SectorUpdates: []sectorUpdate{su},
		})
		if err != nil {
			return build.ExtendErr("failed to add a state change", err)
		}

		// Update the in memory representation of the sector (except the
		// usage), and write the new metadata to disk if needed.
		//
		// The usage is not updated until after the commit has completed to
		// prevent the actual sector data from being overwritten in the event
		// of unclean shutdown.
		if su.Count != 0 {
			wal.cm.sectorLocations[id] = location
			err = wal.writeSectorMetadata(su)
			if err != nil {
				return build.ExtendErr("failed to write sector metadata", err)
			}
		} else {
			delete(wal.cm.sectorLocations, id)
		}

		// Grab the sync channel to know when the update has been durably
		// committed.
		syncChan = wal.syncChan
		return nil
	}()
	if err != nil {
		return build.ExtendErr("cannot remove sector:", err)
	}
	<-syncChan

	// Only update the usage after the sector removal has been committed to
	// disk entirely. The usage is not updated until after the commit has
	// completed to prevent the actual sector data from being overwritten in
	// the event of unclean shutdown.
	if su.Count == 0 {
		wal.mu.Lock()
		defer wal.mu.Unlock()

		sf := wal.cm.storageFolders[su.Folder]
		sf.clearUsage(su.Index)
		sf.sectors--
	}
	return nil
}

// writeSectorMetadata will take a sector update and write the related metadata
// to disk.
func (wal *writeAheadLog) writeSectorMetadata(su sectorUpdate) error {
	sf := wal.cm.storageFolders[su.Folder]
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
	id := cm.managedSectorID(root)

	err := func() error {
		cm.wal.mu.Lock()
		defer cm.wal.mu.Unlock()

		_, exists := cm.sectorLocations[id]
		if exists {
			// Update the location count to indicate that a sector has been
			// added.
			location := wal.cm.sectorLocations[id]
			if location.count == 65535 {
				// Max virtual sectors reached, do not make change.
				return errMaxVirtualSectors
			}
			location.count += 1
			wal.cm.sectorLocations[id] = location
		} else {
			return cm.wal.managedAddPhysicalSector(cm.managedSectorID(root), sectorData)
		}
	}()

	// Write the sector to disk, but only if...
	// Fudge... That 'for' loop makes things way more annoying.

	return wal.managedApplySectorUpdate(sectorUpdate{
		Count:  location.count,
		ID:     id,
		Folder: location.storageFolder,
		Index:  location.index,
	})
}

// DeleteSector will delete a sector from the contract manager. If multiple
// copies of the sector exist, all of them will be removed. This should only be
// used to remove offensive data, as it will cause corruption in the contract
// manager. This corruption puts the contract manager at risk of failing
// storage proofs. If the amount of data removed is small, the risk is small.
// This operation will not destabilize the contract manager.
func (cm *ContractManager) DeleteSector(root crypto.Hash) error {
	return cm.wal.managedDeleteSector(cm.managedSectorID(root))
}

// RemoveSector will remove a sector from the contract manager. If multiple
// copies of the sector exist, only one will be removed.
func (cm *ContractManager) RemoveSector(root crypto.Hash) error {
	return cm.wal.managedRemoveSector(cm.managedSectorID(root))
}
