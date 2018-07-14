package contractmanager

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/fastrand"
)

var (
	// errBadStorageFolderIndex is returned if a storage folder is requested
	// that does not have the correct index.
	errBadStorageFolderIndex = errors.New("no storage folder exists at that index")

	// errIncompleteOffload is returned when the host is tasked with offloading
	// sectors from a storage folder but is unable to offload the requested
	// number - but is able to offload some of them.
	errIncompleteOffload = errors.New("could not successfully offload specified number of sectors from storage folder")

	// errInsufficientRemainingStorageForRemoval is returned if the remaining
	// storage folders do not have enough space remaining to support being
	// removed.
	errInsufficientRemainingStorageForRemoval = errors.New("not enough storage remaining to support removal of disk")

	// errInsufficientRemainingStorageForShrink is returned if the remaining
	// storage folders do not have enough space remaining to support being
	// reduced in size.
	errInsufficientRemainingStorageForShrink = errors.New("not enough storage remaining to support shrinking of disk")

	// ErrLargeStorageFolder is returned if a new storage folder or a resized
	// storage folder would exceed the maximum allowed size.
	ErrLargeStorageFolder = fmt.Errorf("maximum allowed size for a storage folder is %v bytes", MaximumSectorsPerStorageFolder*modules.SectorSize)

	// errMaxStorageFolders indicates that the limit on the number of allowed
	// storage folders has been reached.
	errMaxStorageFolders = fmt.Errorf("host can only accept up to %v storage folders", maximumStorageFolders)

	// errNoFreeSectors is returned if there are no free sectors in the usage
	// array fed to randFreeSector. This error should never be returned, as the
	// contract manager should have sufficient internal consistency to know in
	// advance that there are no free sectors.
	errNoFreeSectors = errors.New("could not find a free sector in the usage array")

	// ErrNoResize is returned if a new size is provided for a storage folder
	// that is the same as the current size of the storage folder.
	ErrNoResize = errors.New("storage folder selected for resize, but new size is same as current size")

	// errRelativePath is returned if a path must be absolute.
	errRelativePath = errors.New("storage folder paths must be absolute")

	// ErrRepeatFolder is returned if a storage folder is added which links to
	// a path that is already in use by another storage folder. Only exact path
	// matches will trigger the error.
	ErrRepeatFolder = errors.New("selected path is already in use as a storage folder, please use 'resize'")

	// ErrSmallStorageFolder is returned if a new storage folder is not large
	// enough to meet the requirements for the minimum storage folder size.
	ErrSmallStorageFolder = fmt.Errorf("minimum allowed size for a storage folder is %v bytes", MinimumSectorsPerStorageFolder*modules.SectorSize)

	// errStorageFolderGranularity is returned if a call to AddStorageFolder
	// tries to use a storage folder size that does not evenly fit into a
	// factor of 8 sectors.
	errStorageFolderGranularity = fmt.Errorf("storage folder must be a factor of %v sectors", storageFolderGranularity)

	// errStorageFolderNotFolder is returned if a storage folder gets added
	// that is not a folder.
	errStorageFolderNotFolder = errors.New("must use an existing folder")

	// errStorageFolderNotFound is returned if a storage folder cannot be
	// found.
	errStorageFolderNotFound = errors.New("could not find storage folder with that id")
)

// storageFolder contains the metadata for a storage folder, including where
// sectors are being stored in the folder. What sectors are being stored is
// managed by the contract manager's sectorLocations map.
type storageFolder struct {
	// mu needs to be RLocked to safetly write new sectors into the storage
	// folder. mu needs to be Locked when the folder is being added, removed,
	// or resized.
	//
	// NOTE: this field must come first in the struct to ensure proper
	// alignment.
	mu sync.TryRWMutex

	// Progress statistics that can be reported to the user. Typically for long
	// running actions like adding or resizing a storage folder.
	atomicProgressNumerator   uint64
	atomicProgressDenominator uint64

	// Disk statistics for this boot cycle.
	atomicFailedReads      uint64
	atomicFailedWrites     uint64
	atomicSuccessfulReads  uint64
	atomicSuccessfulWrites uint64

	// Atomic bool indicating whether or not the storage folder is available. If
	// the storage folder is not available, it will still be loaded but return
	// an error if it is queried.
	atomicUnavailable uint64 // uint64 for alignment

	// The index, path, and usage are all saved directly to disk.
	index uint16
	path  string
	usage []uint64

	// availableSectors indicates sectors which are marked as consumed in the
	// usage field but are actually available. They cannot be marked as free in
	// the usage until the action which freed them has synced to disk, but the
	// settings should mark them as free during syncing.
	//
	// sectors is a count of the number of sectors in use according to the
	// usage field.
	availableSectors map[sectorID]uint32
	sectors          uint64

	// An open file handle is kept so that writes can easily be made to the
	// storage folder without needing to grab a new file handle. This also
	// makes it easy to do delayed-syncing.
	metadataFile modules.File
	sectorFile   modules.File
}

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
	start := fastrand.Intn(len(usage))

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

// usageSectors takes a storage folder usage array and returns a list of active
// sectors in that usage array by their index.
func usageSectors(usage []uint64) (usageSectors []uint32) {
	// Iterate through the usage elements.
	for i, u := range usage {
		// Each usage element corresponds to storageFolderGranularity sectors.
		// Iterate through them and append the ones that are present.
		for j := uint64(0); j < storageFolderGranularity; j++ {
			uMask := uint64(1) << j
			if u&uMask == uMask {
				usageSectors = append(usageSectors, uint32(i)*storageFolderGranularity+uint32(j))
			}
		}
	}
	return usageSectors
}

// vacancyStorageFolder takes a set of storage folders and returns a storage
// folder with vacancy for a sector along with its index. 'nil' and '-1' are
// returned if none of the storage folders are available to accept a sector.
// The returned storage folder will be holding an RLock on its mutex.
func vacancyStorageFolder(sfs []*storageFolder) (*storageFolder, int) {
	enoughRoom := false
	var winningIndex int

	// Go through the folders in random order.
	for _, index := range fastrand.Perm(len(sfs)) {
		sf := sfs[index]

		// Skip past this storage folder if there is not enough room for at
		// least one sector.
		if sf.sectors >= uint64(len(sf.usage))*storageFolderGranularity {
			continue
		}

		// Skip past this storage folder if it's not available to receive new
		// data.
		if !sf.mu.TryRLock() {
			continue
		}

		// Select this storage folder.
		enoughRoom = true
		winningIndex = index
		break
	}
	if !enoughRoom {
		return nil, -1
	}
	return sfs[winningIndex], winningIndex
}

// clearUsage will unset the usage bit at the provided sector index for this
// storage folder.
func (sf *storageFolder) clearUsage(sectorIndex uint32) {
	usageElement := sf.usage[sectorIndex/storageFolderGranularity]
	bitIndex := sectorIndex % storageFolderGranularity
	usageElementUpdated := usageElement & (^(1 << bitIndex))
	if usageElementUpdated != usageElement {
		sf.sectors--
		sf.usage[sectorIndex/storageFolderGranularity] = usageElementUpdated
	}
}

// setUsage will set the usage bit at the provided sector index for this
// storage folder.
func (sf *storageFolder) setUsage(sectorIndex uint32) {
	usageElement := sf.usage[sectorIndex/storageFolderGranularity]
	bitIndex := sectorIndex % storageFolderGranularity
	usageElementUpdated := usageElement | (1 << bitIndex)
	if usageElementUpdated != usageElement {
		sf.sectors++
		sf.usage[sectorIndex/storageFolderGranularity] = usageElementUpdated
	}
}

// availableStorageFolders returns the contract manager's storage folders as a
// slice, excluding any unavailable storeage folders.
func (cm *ContractManager) availableStorageFolders() []*storageFolder {
	sfs := make([]*storageFolder, 0)
	for _, sf := range cm.storageFolders {
		// Skip unavailable storage folders.
		if atomic.LoadUint64(&sf.atomicUnavailable) == 1 {
			continue
		}
		sfs = append(sfs, sf)
	}
	return sfs
}

// threadedFolderRecheck checks the unavailable storage folders and looks to see
// if they have been mounted or restored by the user.
func (cm *ContractManager) threadedFolderRecheck() {
	// Don't spawn the loop if 'noRecheck' disruption is set.
	if cm.dependencies.Disrupt("noRecheck") {
		return
	}

	sleepTime := folderRecheckInitialInterval
	for {
		// Check for shutdown.
		select {
		case <-cm.tg.StopChan():
			return
		case <-time.After(sleepTime):
		}

		// Check all of the storage folders and recover any that have been added
		// to the contract manager.
		cm.wal.mu.Lock()
		for _, sf := range cm.storageFolders {
			if atomic.LoadUint64(&sf.atomicUnavailable) == 1 {
				var err1, err2 error
				sf.metadataFile, err1 = cm.dependencies.OpenFile(filepath.Join(sf.path, metadataFile), os.O_RDWR, 0700)
				sf.sectorFile, err2 = cm.dependencies.OpenFile(filepath.Join(sf.path, sectorFile), os.O_RDWR, 0700)
				if err1 == nil && err2 == nil {
					// The storage folder has been found, and loading can be
					// completed.
					cm.loadSectorLocations(sf)
				} else {
					// One of the opens failed, close the file handle for the
					// opens that did not fail.
					if err1 == nil {
						sf.metadataFile.Close()
					}
					if err2 == nil {
						sf.sectorFile.Close()
					}
				}
			}
		}
		cm.wal.mu.Unlock()

		// Increase the sleep time.
		if sleepTime*2 < maxFolderRecheckInterval {
			sleepTime *= 2
		}
	}
}

// ResetStorageFolderHealth will reset the read and write statistics for the
// input storage folder.
func (cm *ContractManager) ResetStorageFolderHealth(index uint16) error {
	err := cm.tg.Add()
	if err != nil {
		return err
	}
	defer cm.tg.Done()
	cm.wal.mu.Lock()
	defer cm.wal.mu.Unlock()

	sf, exists := cm.storageFolders[index]
	if !exists {
		return errStorageFolderNotFound
	}
	atomic.StoreUint64(&sf.atomicFailedReads, 0)
	atomic.StoreUint64(&sf.atomicFailedWrites, 0)
	atomic.StoreUint64(&sf.atomicSuccessfulReads, 0)
	atomic.StoreUint64(&sf.atomicSuccessfulWrites, 0)
	return nil
}

// ResizeStorageFolder will resize a storage folder, moving sectors as
// necessary. The resize operation will stop and return an error if any of the
// sector move operations fail. If the force flag is set to true, the resize
// operation will continue through failures, meaning that data will be lost.
func (cm *ContractManager) ResizeStorageFolder(index uint16, newSize uint64, force bool) error {
	err := cm.tg.Add()
	if err != nil {
		return err
	}
	defer cm.tg.Done()

	cm.wal.mu.Lock()
	sf, exists := cm.storageFolders[index]
	cm.wal.mu.Unlock()
	if !exists || atomic.LoadUint64(&sf.atomicUnavailable) == 1 {
		return errStorageFolderNotFound
	}

	if newSize/modules.SectorSize < MinimumSectorsPerStorageFolder {
		return ErrSmallStorageFolder
	}
	if newSize/modules.SectorSize > MaximumSectorsPerStorageFolder {
		return ErrLargeStorageFolder
	}

	oldSize := uint64(len(sf.usage)) * storageFolderGranularity * modules.SectorSize
	if oldSize == newSize {
		return ErrNoResize
	}
	newSectorCount := uint32(newSize / modules.SectorSize)
	if oldSize > newSize {
		return cm.wal.shrinkStorageFolder(index, newSectorCount, force)
	}
	return cm.wal.growStorageFolder(index, newSectorCount)
}

// StorageFolders will return a list of storage folders in the host, each
// containing information about the storage folder and any operations currently
// being executed on the storage folder.
func (cm *ContractManager) StorageFolders() []modules.StorageFolderMetadata {
	err := cm.tg.Add()
	if err != nil {
		return nil
	}
	defer cm.tg.Done()
	cm.wal.mu.Lock()
	defer cm.wal.mu.Unlock()

	// Iterate over the storage folders that are in memory first, and then
	// suppliment them with the storage folders that are not in memory.
	var smfs []modules.StorageFolderMetadata
	for _, sf := range cm.storageFolders {
		// Grab the non-computational data.
		sfm := modules.StorageFolderMetadata{
			ProgressNumerator:   atomic.LoadUint64(&sf.atomicProgressNumerator),
			ProgressDenominator: atomic.LoadUint64(&sf.atomicProgressDenominator),

			FailedReads:      atomic.LoadUint64(&sf.atomicFailedReads),
			FailedWrites:     atomic.LoadUint64(&sf.atomicFailedWrites),
			SuccessfulReads:  atomic.LoadUint64(&sf.atomicSuccessfulReads),
			SuccessfulWrites: atomic.LoadUint64(&sf.atomicSuccessfulWrites),

			Capacity:          modules.SectorSize * 64 * uint64(len(sf.usage)),
			CapacityRemaining: ((64 * uint64(len(sf.usage))) - sf.sectors) * modules.SectorSize,
			Index:             sf.index,
			Path:              sf.path,
		}

		// Set some of the values to extreme numbers if the storage folder is
		// unavailable, to flag the user's attention.
		if atomic.LoadUint64(&sf.atomicUnavailable) == 1 {
			sfm.FailedReads = 9999999999
			sfm.FailedWrites = 9999999999
		}

		// Add this storage folder to the list of storage folders.
		smfs = append(smfs, sfm)
	}
	return smfs
}
