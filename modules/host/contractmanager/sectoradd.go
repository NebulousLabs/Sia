package contractmanager

import (
	"encoding/binary"
	"errors"
	"math"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
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
// the usage array. If one is found, the bit will be flipped atomically in the
// usage array to indicate that the sector is no longer available. The uint64
// indicates the index of the sector within the usage array.
func randFreeSector(usage []uint64) uint32 {
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

		// If nothing was found even after scanning the front of the array,
		// panic as this function should not be called with a full usage array.
		if i == start {
			panic("unable to find an empty sector in the usage array")
		}
	}

	// Get the most significant zero. This is achieved by performing a 'most
	// significant bit' on the XOR of the actual value. Set that bit, and
	// return the index of the sector that has been selected.
	msb := mostSignificantBit(^usage[i])
	usage[i] = usage[i] + (1 << msb)
	return uint32((uint64(i) * 64) + msb)
}

// managedAddSector is a WAL operation to add a sector to the contract manager.
func (wal *writeAheadLog) managedAddSector(id sectorID, data []byte) error {
	var syncChan chan struct{}
	err := func() error {
		wal.mu.Lock()
		defer wal.mu.Unlock()

		// Create and fill out the sectorAdd object.
		sa := sectorAdd{
			Data: data,
			ID:   id,
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

			// All that needs to happen is the count should be incremented and
			// updated, and the sectorAdd should be filled out.
			sa.Count = location.count + 1
			sa.Folder = location.storageFolder
			sa.Index = location.index

			// Data can be wiped, as the data is already there, there's no need
			// to commit to the data or to write it to disk.
			sa.Data = nil

			// Update the location count to indicate that a sector has been
			// added.
			location.count += 1
			wal.cm.sectorLocations[id] = location
		} else {
			// Sanity check - data should have modules.SectorSize bytes.
			if uint64(len(data)) != modules.SectorSize {
				wal.cm.log.Critical("sector has the wrong size", modules.SectorSize, len(data))
				return errors.New("malformed sector")
			}

			// Find a committed storage folder that has enough space to receive
			// this sector.
			sfs := wal.storageFolders()
			sf, _ := emptiestStorageFolder(sfs)
			if sf == nil {
				// None of the storage folders have enough room to house the
				// sector.
				return errInsufficientStorageForSector
			}

			// Find a location for the sector within the file using the Usage
			// field.
			sectorIndex := randFreeSector(sf.Usage)

			// Set the sectorAdd fields and update the sectorLocations map to
			// reflect this new sector.
			sa.Count = 1
			sa.Folder = sf.Index
			sa.Index = sectorIndex
			wal.cm.sectorLocations[id] = sectorLocation{
				index:         sectorIndex,
				storageFolder: sf.Index,
				count:         1,
			}
			sf.Sectors += 1
		}

		// Add a change to the WAL to commit this sector to the provided index.
		err := wal.appendChange(stateChange{
			AddedSectors: []sectorAdd{sa},
		})
		if err != nil {
			return build.ExtendErr("failed to add a state change", err)
		}

		// Grab the synchronization channel so that we know when the sector add
		// has completed.
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

// commitAddSector will commit a sector that has been added to the WAL. The
// commit should be idempotent, meaning if this function is run multiple times
// on the same sector, there should be no issues.
func (wal *writeAheadLog) commitAddSector(sa sectorAdd) {
	sf := wal.cm.storageFolders[sa.Folder]
	lookupTableSize := len(sf.Usage) * storageFolderGranularity * sectorMetadataDiskSize

	// Write the sector metadata to disk.
	writeData := make([]byte, sectorMetadataDiskSize)
	copy(writeData, sa.ID[:])
	binary.LittleEndian.PutUint16(writeData[12:], sa.Count)
	_, err := sf.file.Seek(sectorMetadataDiskSize*int64(sa.Index), 0)
	if err != nil {
		wal.cm.log.Println("ERROR: unable to seek to sector metadata when adding sector")
		sf.failedWrites += 1
		return
	}
	_, err = sf.file.Write(writeData)
	if err != nil {
		wal.cm.log.Println("ERROR: unable to write sector metadata when adding sector")
		sf.failedWrites += 1
		return
	}

	// Write the sector to disk. The write only needs to happen if the count is
	// equal to 1, otherwise the data can be assumed to already be there.
	if sa.Count == 1 {
		_, err = sf.file.Seek(int64(modules.SectorSize)*int64(sa.Index)+int64(lookupTableSize), 0)
		if err != nil {
			wal.cm.log.Println("ERROR: unable to seek to sector data when adding sector")
			sf.failedWrites += 1
			return
		}
		_, err = sf.file.Write(sa.Data)
		if err != nil {
			wal.cm.log.Println("ERROR: unable to write sector data when adding sector")
			sf.failedWrites += 1
			return
		}
	}

	// Writes were successful, update the storage folder stats.
	sf.successfulWrites += 1
}

// AddSector will add a sector to the contract manager.
func (cm *ContractManager) AddSector(root crypto.Hash, sectorData []byte) error {
	return cm.wal.managedAddSector(cm.managedSectorID(root), sectorData)
}
