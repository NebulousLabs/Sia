package contractmanager

import (
	"math"
	"sync/atomic"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
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
	}
	// If nothing was found even after scanning the front of the array, panic
	// as this function should not be called with a full usage array.
	if i == start {
		panic("unable to find an empty sector in the usage array")
	}

	// Get the most significant zero. This is achieved by performing a 'most
	// significant bit' on the XOR of the actual value.
	msb := mostSignificantBit(^usage[i])
	// Before returning, we want to set that bit.
	newUsage := usage[i] + (1 << msb)
	atomic.StoreUint64(&usage[i], newUsage)

	// Calculate and return the index of the free sector.
	return uint32((i * 64) + msb)
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
			ID: id,
		}

		// Grab the number of virtual sectors that have been committed with
		// this root.
		location, exists := wal.cm.sectorLocations[id]
		if exists {
			// All that needs to happen is the count should be incremented and
			// updated, and the sectorAdd should be filled out.
			sa.Count = location.count + 1
			sa.Folder = location.storageFolder
			sa.Index = location.index

			// Update the location count to indicate that a sector has been
			// added.
			location.count += 1
			wal.cm.sectorLocations[id] = location
		} else {
			// Find a committed storage folder that has enough space to receive
			// this sector.
			//
			// TODO: this search needs to consider storage folders that are in
			// the process of being removed, and should avoid any storage
			// folders which are undergoing a remove operation which is
			// incomplete. This would look like a pre-process which compares
			// the list of committed storage folders to the list of uncommitted
			// removals, before passing the result to emptiestStorageFolder.
			sf, _ := emptiestStorageFolder(wal.cm.storageFolders)
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
				index: sectorIndex,
				storageFolder: sf.Index,
				count: 1,
			}
		}

		// Add a change to the WAL to commit this sector to the provided index.
		err = wa.appendChange(stateChange{
			AddedSctors: []sectorAdd{sa},
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
	// TODO TODO TODO: pick up here. Don't forget that your memory model has
	// evolved, and that it will allow you to pull a pretty significant amount
	// of code from AddStorageFolder.
}

// AddSector will add a sector to the contract manager.
func (cm *ContractManager) AddSector(root crypto.Hash, sectorData []byte) {
	return managedAddSector(cm.managedSectorID(root), sectorData)
}
