package contractmanager

import (
	"encoding/binary"
	"errors"
	"sync"
	"sync/atomic"

	"gitlab.com/NebulousLabs/Sia/build"
	"gitlab.com/NebulousLabs/Sia/crypto"
	"gitlab.com/NebulousLabs/Sia/modules"
)

var (
	// errDiskTrouble is returned when the host is supposed to have enough
	// storage to hold a new sector but failures that are likely related to the
	// disk have prevented the host from successfully adding the sector.
	errDiskTrouble = errors.New("host unable to add sector despite having the storage capacity to do so")

	// errInsufficientStorageForSector is returned if the host tries to add a
	// sector when there is not enough storage remaining on the host to accept
	// the sector.
	//
	// Ideally, the host will adjust pricing as the host starts to fill up, so
	// this error should be pretty rare. Demand should drive the price up
	// faster than the Host runs out of space, such that the host is always
	// hovering around 95% capacity and rarely over 98% or under 90% capacity.
	errInsufficientStorageForSector = errors.New("not enough storage remaining to accept sector")

	// errMaxVirtualSectors is returned when a sector cannot be added because
	// the maximum number of virtual sectors for that sector id already exist.
	errMaxVirtualSectors = errors.New("sector collides with a physical sector that already has the maximum allowed number of virtual sectors")

	// ErrSectorNotFound is returned when a lookup for a sector fails.
	ErrSectorNotFound = errors.New("could not find the desired sector")
)

// sectorLocation indicates the location of a sector on disk.
type (
	sectorID [12]byte

	sectorLocation struct {
		// index indicates the index of the sector's location within the storage
		// folder.
		index uint32

		// storageFolder indicates the index of the storage folder that the sector
		// is stored on.
		storageFolder uint16

		// count indicates the number of virtual sectors represented by the
		// physical sector described by this object. A maximum of 2^16 virtual
		// sectors are allowed for each sector. Proper use by the renter should
		// mean that the host never has more than 3 virtual sectors for any sector.
		count uint16
	}

	// sectorLock contains a lock plus a count of the number of threads
	// currently waiting to access the lock.
	sectorLock struct {
		waiting int
		mu      sync.Mutex
	}
)

// readSector will read the sector in the file, starting from the provided
// location.
func readSector(f modules.File, sectorIndex uint32) ([]byte, error) {
	b := make([]byte, modules.SectorSize)
	_, err := f.ReadAt(b, int64(uint64(sectorIndex)*modules.SectorSize))
	if err != nil {
		return nil, build.ExtendErr("unable to read within storage folder", err)
	}
	return b, nil
}

// readFullMetadata will read a full sector metadata file into memory.
func readFullMetadata(f modules.File, numSectors int) ([]byte, error) {
	sectorLookupBytes := make([]byte, numSectors*sectorMetadataDiskSize)
	_, err := f.ReadAt(sectorLookupBytes, 0)
	if err != nil {
		return nil, build.ExtendErr("unable to read metadata file for target storage folder", err)
	}
	return sectorLookupBytes, nil
}

// writeSector will write the given sector into the given file at the given
// index.
func writeSector(f modules.File, sectorIndex uint32, data []byte) error {
	_, err := f.WriteAt(data, int64(uint64(sectorIndex)*modules.SectorSize))
	if err != nil {
		return build.ExtendErr("unable to write within provided file", err)
	}
	return nil
}

// writeSectorMetadata will take a sector update and write the related metadata
// to disk.
func writeSectorMetadata(f modules.File, sectorIndex uint32, id sectorID, count uint16) error {
	writeData := make([]byte, sectorMetadataDiskSize)
	copy(writeData, id[:])
	binary.LittleEndian.PutUint16(writeData[12:], count)
	_, err := f.WriteAt(writeData, sectorMetadataDiskSize*int64(sectorIndex))
	if err != nil {
		return build.ExtendErr("unable to write in given file", err)
	}
	return nil
}

// sectorID returns the id that should be used when referring to a sector.
// There are lots of sectors, and to minimize their footprint a reduced size
// hash is used. Hashes are typically 256bits to provide collision resistance
// when an attacker can perform orders of magnitude more than a billion trials
// per second. When attacking the host sector ids though, the attacker can only
// do one trial per sector upload, and even then has minimal means to learn
// whether or not a collision was successfully achieved. Hash length can safely
// be reduced from 32 bytes to 12 bytes, which has a collision resistance of
// 2^48. The host however is unlikely to be storing 2^48 sectors, which would
// be an exabyte of data.
func (cm *ContractManager) managedSectorID(sectorRoot crypto.Hash) (id sectorID) {
	saltedRoot := crypto.HashAll(sectorRoot, cm.sectorSalt)
	copy(id[:], saltedRoot[:])
	return id
}

// ReadSector will read a sector from the storage manager, returning the bytes
// that match the input sector root.
func (cm *ContractManager) ReadSector(root crypto.Hash) ([]byte, error) {
	err := cm.tg.Add()
	if err != nil {
		return nil, err
	}
	defer cm.tg.Done()
	id := cm.managedSectorID(root)
	cm.wal.managedLockSector(id)
	defer cm.wal.managedUnlockSector(id)

	// Fetch the sector metadata.
	cm.wal.mu.Lock()
	sl, exists1 := cm.sectorLocations[id]
	sf, exists2 := cm.storageFolders[sl.storageFolder]
	cm.wal.mu.Unlock()
	if !exists1 {
		return nil, ErrSectorNotFound
	}
	if !exists2 {
		cm.log.Critical("Unable to load storage folder despite having sector metadata")
		return nil, ErrSectorNotFound
	}
	if atomic.LoadUint64(&sf.atomicUnavailable) == 1 {
		// TODO: Pick a new error instead.
		return nil, ErrSectorNotFound
	}

	// Read the sector.
	sectorData, err := readSector(sf.sectorFile, sl.index)
	if err != nil {
		atomic.AddUint64(&sf.atomicFailedReads, 1)
		return nil, build.ExtendErr("unable to fetch sector", err)
	}
	atomic.AddUint64(&sf.atomicSuccessfulReads, 1)
	return sectorData, nil
}

// managedLockSector grabs a sector lock.
func (wal *writeAheadLog) managedLockSector(id sectorID) {
	wal.mu.Lock()
	sl, exists := wal.cm.lockedSectors[id]
	if exists {
		sl.waiting++
	} else {
		sl = &sectorLock{
			waiting: 1,
		}
		wal.cm.lockedSectors[id] = sl
	}
	wal.mu.Unlock()

	// Block until the sector is available.
	sl.mu.Lock()
}

// managedUnlockSector releases a sector lock.
func (wal *writeAheadLog) managedUnlockSector(id sectorID) {
	wal.mu.Lock()
	defer wal.mu.Unlock()

	// Release the lock on the sector.
	sl, exists := wal.cm.lockedSectors[id]
	if !exists {
		wal.cm.log.Critical("Unlock of sector that is not locked.")
		return
	}
	sl.waiting--
	sl.mu.Unlock()

	// If nobody else is trying to lock the sector, perform garbage collection.
	if sl.waiting == 0 {
		delete(wal.cm.lockedSectors, id)
	}
}
