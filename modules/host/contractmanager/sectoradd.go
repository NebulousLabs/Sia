package contractmanager

import (
	"encoding/binary"
	"errors"
	"math"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

var (
	// errNoFreeSectors is returned if there are no free sectors in the usage
	// array fed to randFreeSector. This error should never be returned, as the
	// contract manager should have sufficent internal consistency to know in
	// advance that there are no free sectors.
	errNoFreeSectors = errors.New("could not find a free sector in the usage array")
)

// mostSignificantBit returns the index of the most significant bit of an input
// value.
func mostSignificantBit(i uint64) uint64 {
	if i == 0 {
		panic("no bits set in input")
	}

	bval := []uint64{0, 0, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3, 3, 3, 3, 3, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7}
	r := uint64(0)
	if i&0xffffffff00000000 != 0 {
		r += 32
		i = i >> 32
	}
	if i&0x00000000ffff0000 != 0 {
		r += 16
		i = i >> 16
	}
	if i&0x000000000000ff00 != 0 {
		r += 8
		i = i >> 8
	}
	if i&0x00000000000000f0 != 0 {
		r += 4
		i = i >> 4
	}
	return r + bval[i]
}

// randFreeSector will take a usage array and find a random free sector within
// the usage array. The uint32 indicates the index of the sector within the
// usage array.
func randFreeSector(usage []uint64) (uint32, error) {
	// Pick a random starting location. Scanning the sector in a short amount
	// of time requires starting from a random place.
	start, err := crypto.RandIntn(len(usage))
	if err != nil {
		panic(err)
	}

	// Find the first element of the array that is not completely full.
	var i int
	for i = start; i < len(usage); i++ {
		if usage[i] != math.MaxUint64 {
			break
		}
	}
	// If nothing was found by the end of the array, a wraparound is needed.
	if i == len(usage) {
		for i = 0; i < start; i++ {
			if usage[i] != math.MaxUint64 {
				break
			}
		}
		// Return an error if no empty sectors were found.
		if i == start {
			return 0, errNoFreeSectors
		}
	}

	// Get the most significant zero. This is achieved by performing a 'most
	// significant bit' on the XOR of the actual value. Return the index of the
	// sector that has been selected.
	msz := mostSignificantBit(^usage[i])
	return uint32((uint64(i) * 64) + msz), nil
}

// managedAddSector is a WAL operation to add a sector to the contract manager.
func (wal *writeAheadLog) managedAddSector(id sectorID, data []byte) error {
	var syncChan chan struct{}
	err := func() error {
		wal.mu.Lock()
		defer wal.mu.Unlock()

		// Create and fill out the sectorUpdate object.
		su := sectorUpdate{
			ID: id,
		}

		// Grab the number of virtual sectors that have been committed with
		// this root.
		location, exists := wal.cm.sectorLocations[id]
		if exists {
			// Check whether the maximum number of virtual sectors has been
			// reached.
			if location.count == 65535 {
				return errMaxVirtualSectors
			}

			// Update the location count to indicate that a sector has been
			// added.
			location.count += 1
			wal.cm.sectorLocations[id] = location

			// Fill out the sectorUpdate object so that it can be added to the
			// WAL.
			su.Count = location.count
			su.Folder = location.storageFolder
			su.Index = location.index
		} else {
			// Sanity check - data should have modules.SectorSize bytes.
			if uint64(len(data)) != modules.SectorSize {
				wal.cm.log.Critical("sector has the wrong size", modules.SectorSize, len(data))
				return errors.New("malformed sector")
			}

			// Find a committed storage folder that has enough space to receive
			// this sector.
			sf, _ := emptiestStorageFolder(wal.cm.storageFolderSlice())
			if sf == nil {
				// None of the storage folders have enough room to house the
				// sector.
				return errInsufficientStorageForSector
			}

			// Find a location for the sector within the file using the Usage
			// field.
			sectorIndex, err := randFreeSector(sf.Usage)
			if err != nil {
				wal.cm.log.Critical("a storage folder with full usage was returned from emptiestStorageFolder")
				return err
			}

			// Write the new sector to disk. Any data existing in this location
			// on disk is either garbage or is from a sector that has been
			// removed through a successfully committed remove operation - no
			// risk of corruption to write immediately.
			//
			// If the commitment to update the metadata fails, the host will
			// never know that the sector existed on-disk and will treat it as
			// garbage data - which does not threaten consistency.
			lookupTableSize := len(sf.Usage) * storageFolderGranularity * sectorMetadataDiskSize
			_, err = sf.file.Seek(int64(modules.SectorSize)*int64(sectorIndex)+int64(lookupTableSize), 0)
			if err != nil {
				wal.cm.log.Println("ERROR: unable to seek to sector data when adding sector")
				sf.failedWrites += 1
				return errDiskTrouble
			}
			_, err = sf.file.Write(data)
			if err != nil {
				wal.cm.log.Println("ERROR: unable to write sector data when adding sector")
				sf.failedWrites += 1
				return errDiskTrouble
			}

			// Update the state to reflect the new sector.
			wal.cm.sectorLocations[id] = sectorLocation{
				index:         sectorIndex,
				storageFolder: sf.Index,
				count:         1,
			}
			sf.sectors += 1

			// Update the usage field in the storage folder.
			usageElement := sf.Usage[sectorIndex/storageFolderGranularity]
			bitIndex := sectorIndex % storageFolderGranularity
			usageElement = usageElement | (1 << bitIndex)
			sf.Usage[sectorIndex/storageFolderGranularity] = usageElement

			// Fill out the sectorUpdate fields.
			su.Count = 1
			su.Folder = sf.Index
			su.Index = sectorIndex
		}

		// Write the sector metadata to disk.
		err := wal.writeSectorMetadata(su)
		if err != nil {
			return build.ExtendErr("unable to write sector metadata during addSector call", err)
		}

		// Add a change to the WAL to commit this sector to the provided index.
		err = wal.appendChange(stateChange{
			SectorUpdates: []sectorUpdate{su},
		})
		if err != nil {
			return build.ExtendErr("failed to add a state change", err)
		}

		// Grab the synchronization channel so that we know when the sector add
		// has been committed.
		syncChan = wal.syncChan
		return nil
	}()
	if err != nil {
		return build.ExtendErr("verification failed:", err)
	}

	// Only return after the commitment has succeeded fully.
	<-syncChan
	return nil
}

// commitUpdateSector will commit a sector update to the contract manager,
// writing in metadata and usage info if the sector still exists, and deleting
// the usage info if the sector does not exist. The update is idempotent.
func (wal *writeAheadLog) commitUpdateSector(su sectorUpdate) {
	// Grab the usage flag, as it will need to be updated.
	usageElement := wal.cm.storageFolders[su.Folder].Usage[su.Index/storageFolderGranularity]
	bitIndex := su.Index % storageFolderGranularity

	// If the sector is being cleaned from disk, unset the usage flag. No need
	// to update the metadata, the contractor now sees it as garbage data
	// anyway.
	if su.Count == 0 {
		usageElement = usageElement & (^(1 << bitIndex))
		wal.cm.storageFolders[su.Folder].Usage[su.Index/storageFolderGranularity] = usageElement
		return
	}

	// If the sector is not being purged, set the usage flag.
	usageElement = usageElement | (1 << bitIndex)
	wal.cm.storageFolders[su.Folder].Usage[su.Index/storageFolderGranularity] = usageElement

	// Write the updated sector metadata to disk. The sector itself will
	// already have been written to disk and synced.
	wal.writeSectorMetadata(su)
}

// writeSectorMetadata will take a sector update and write the related metadata
// to disk.
func (wal *writeAheadLog) writeSectorMetadata(su sectorUpdate) error {
	sf := wal.cm.storageFolders[su.Folder]
	writeData := make([]byte, sectorMetadataDiskSize)
	copy(writeData, su.ID[:])
	binary.LittleEndian.PutUint16(writeData[12:], su.Count)
	_, err := sf.file.Seek(sectorMetadataDiskSize*int64(su.Index), 0)
	if err != nil {
		wal.cm.log.Println("ERROR: unable to seek to sector metadata when adding sector")
		sf.failedWrites += 1
		return err
	}
	_, err = sf.file.Write(writeData)
	if err != nil {
		wal.cm.log.Println("ERROR: unable to write sector metadata when adding sector")
		sf.failedWrites += 1
		return err
	}
	// Writes were successful, update the storage folder stats.
	sf.successfulWrites += 1
	return nil
}

// AddSector will add a sector to the contract manager.
func (cm *ContractManager) AddSector(root crypto.Hash, sectorData []byte) error {
	return cm.wal.managedAddSector(cm.managedSectorID(root), sectorData)
}
