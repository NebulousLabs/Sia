package contractmanager

import (
	"encoding/binary"
	"errors"
	"sync/atomic"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
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

	// errSectorNotFound is returned when a lookup for a sector fails.
	errSectorNotFound = errors.New("could not find the desired sector")
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
		// phsyical sector described by this object. A maximum of 2^16 virtual
		// sectors are allowed for each sector. Proper use by the renter should
		// mean that the host never has more than 3 virtual sectors for any sector.
		count uint16
	}
)

// readSector will read the sector in the file, starting from the provided
// location.
func readSector(f file, sectorIndex uint32) ([]byte, error) {
	f.Lock()
	defer f.Unlock()

	b := make([]byte, modules.SectorSize)
	_, err := f.Seek(int64(uint64(sectorIndex)*modules.SectorSize), 0)
	if err != nil {
		return nil, build.ExtendErr("unable to seek within storage folder", err)
	}
	_, err = f.Read(b)
	if err != nil {
		return nil, build.ExtendErr("unable to read within storage folder", err)
	}
	return b, nil
}

// readFullMetadata will read a full sector metadata file into memory.
func readFullMetadata(f file, numSectors int) ([]byte, error) {
	f.Lock()
	defer f.Unlock()

	sectorLookupBytes := make([]byte, numSectors*sectorMetadataDiskSize)
	_, err := f.Seek(0, 0)
	if err != nil {
		return nil, build.ExtendErr("unable to seek through metadata file for target storage folder", err)
	}
	_, err = f.Read(sectorLookupBytes)
	if err != nil {
		return nil, build.ExtendErr("unable to read metadata file for target storage folder", err)
	}
	return sectorLookupBytes, nil
}

// writeSector will write the given sector into the given file at the given
// index.
func writeSector(f file, sectorIndex uint32, data []byte) error {
	f.Lock()
	defer f.Unlock()

	_, err := f.Seek(int64(uint64(sectorIndex)*modules.SectorSize), 0)
	if err != nil {
		return build.ExtendErr("unable to seek within provided file", err)
	}
	_, err = f.Write(data)
	if err != nil {
		return build.ExtendErr("unable to read within provided file", err)
	}
	return nil
}

// writeSectorMetadata will take a sector update and write the related metadata
// to disk.
func writeSectorMetadata(f file, sectorIndex uint32, id sectorID, count uint16) error {
	f.Lock()
	defer f.Unlock()

	writeData := make([]byte, sectorMetadataDiskSize)
	copy(writeData, id[:])
	binary.LittleEndian.PutUint16(writeData[12:], count)
	_, err := f.Seek(sectorMetadataDiskSize*int64(sectorIndex), 0)
	if err != nil {
		return build.ExtendErr("unable to seek in given file", err)
	}
	_, err = f.Write(writeData)
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
	// Fetch the sector metadata.
	id := cm.managedSectorID(root)
	cm.wal.mu.Lock()
	sl, exists1 := cm.sectorLocations[id]
	sf, exists2 := cm.storageFolders[sl.storageFolder]
	cm.wal.mu.Unlock()
	if !exists1 {
		return nil, errSectorNotFound
	}
	if !exists2 {
		cm.log.Critical("Unable to load storage folder despite having sector metadata")
		return nil, errSectorNotFound
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
