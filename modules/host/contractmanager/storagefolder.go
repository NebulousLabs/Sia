package contractmanager

// storagefolders.go is responsible for managing the folders/files that contain
// the sectors within the contract manager. Storage folders can be added,
// resized, or removed.

// TODO: Need to make sure that the sector count accounting (sf.sectors) in the
// storage folder is accurate - it should probably align with the usage, though
// setting and clearing the usage during idempotent actions could cause
// problems. Idempotent actions though I believe only apply when doing
// recovery. (that's how it should be designed at least)
//
// ergo, set + clear should modify the total number of sectors?

import (
	"errors"
	"fmt"
	"math"
	"sync/atomic"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/sync"
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

	// errLargeStorageFolder is returned if a new storage folder or a resized
	// storage folder would exceed the maximum allowed size.
	errLargeStorageFolder = fmt.Errorf("maximum allowed size for a storage folder is %v bytes", maximumSectorsPerStorageFolder*modules.SectorSize)

	// errMaxStorageFolders indicates that the limit on the number of allowed
	// storage folders has been reached.
	errMaxStorageFolders = fmt.Errorf("host can only accept up to %v storage folders", maximumStorageFolders)

	// errNoFreeSectors is returned if there are no free sectors in the usage
	// array fed to randFreeSector. This error should never be returned, as the
	// contract manager should have sufficent internal consistency to know in
	// advance that there are no free sectors.
	errNoFreeSectors = errors.New("could not find a free sector in the usage array")

	// errNoResize is returned if a new size is provided for a storage folder
	// that is the same as the current size of the storage folder.
	errNoResize = errors.New("storage folder selected for resize, but new size is same as current size")

	// errRepeatFolder is returned if a storage folder is added which links to
	// a path that is already in use by another storage folder. Only exact path
	// matches will trigger the error.
	errRepeatFolder = errors.New("selected path is already in use as a storage folder, please use 'resize'")

	// errSmallStorageFolder is returned if a new storage folder is not large
	// enough to meet the requirements for the minimum storage folder size.
	errSmallStorageFolder = fmt.Errorf("minimum allowed size for a storage folder is %v bytes", minimumSectorsPerStorageFolder*modules.SectorSize)

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

	// errRelativePath is returned if a path must be absolute.
	errRelativePath = errors.New("storage folder paths must be absolute")
)

// storageFolder contains the metadata for a storage folder, including where
// sectors are being stored in the folder. What sectors are being stored is
// managed by the contract manager's sectorLocations map.
type storageFolder struct {
	// Certain operations on a storage folder can take a long time (Add,
	// Remove, and Resize). The fields below indicate progress on any long
	// running action being performed on them. Determining which action is
	// being performed can be determined using the context of the WAL state
	// changes that are currently open.
	atomicProgressNumerator   uint64
	atomicProgressDenominator uint64

	// Disk statistics for this boot cycle.
	atomicFailedReads      uint64
	atomicFailedWrites     uint64
	atomicSuccessfulReads  uint64
	atomicSuccessfulWrites uint64

	// The index, path, and usage are all saved directly to disk.
	index uint16
	path  string
	usage []uint64

	// queuedSectors contains a list of sectors which have been queued to be
	// added to the storage folder where the add has not yet completed. TODO:
	// Better to consider it 'queuedUsage' - usage available canonically, but
	// not actively to avoid race condition.
	//
	// sectors is a running tally of the number of physical sectors in the
	// storage folder.
	queuedSectors map[sectorID]uint32
	sectors       uint64

	// mu needs to be RLocked to safetly write new sectors into the storage
	// folder. mu needs to be Locked when the folder is being resized or
	// manipulated.
	mu sync.TryRWMutex

	// An open file handle is kept so that writes can easily be made to the
	// storage folder without needing to grab a new file handle. This also
	// makes it easy to do delayed-syncing.
	metadataFile file
	sectorFile   file
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
	ordering, err := crypto.Perm(len(sfs))
	if err != nil {
		panic(err)
	}
	for _, index := range ordering {
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

// storageFolderSlice returns the contract manager's storage folders map as a
// slice.
func (cm *ContractManager) storageFolderSlice() []*storageFolder {
	sfs := make([]*storageFolder, 0)
	for _, sf := range cm.storageFolders {
		sfs = append(sfs, sf)
	}
	return sfs
}

// ResetStorageFolderHealth will reset the read and write statistics for the
// input storage folder.
func (cm *ContractManager) ResetStorageFolderHealth(index uint16) error {
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

// StorageFolders will return a list of storage folders in the host, each
// containing information about the storage folder and any operations currently
// being executed on the storage folder.
func (cm *ContractManager) StorageFolders() []modules.StorageFolderMetadata {
	cm.wal.mu.Lock()
	defer cm.wal.mu.Unlock()

	// Iterate over the storage folders that are in memory first, and then
	// suppliment them with the storage folders that are not in memory.
	var smfs []modules.StorageFolderMetadata
	for _, sf := range cm.storageFolders {
		// Grab the non-computational data.
		sfm := modules.StorageFolderMetadata{
			Capacity:          modules.SectorSize * 64 * uint64(len(sf.usage)),
			CapacityRemaining: ((64 * uint64(len(sf.usage))) - sf.sectors) * modules.SectorSize,
			Index:             sf.index,
			Path:              sf.path,

			ProgressNumerator:   atomic.LoadUint64(&sf.atomicProgressNumerator),
			ProgressDenominator: atomic.LoadUint64(&sf.atomicProgressDenominator),
		}
		sfm.FailedReads = atomic.LoadUint64(&sf.atomicFailedReads)
		sfm.FailedWrites = atomic.LoadUint64(&sf.atomicFailedWrites)
		sfm.SuccessfulReads = atomic.LoadUint64(&sf.atomicSuccessfulReads)
		sfm.SuccessfulWrites = atomic.LoadUint64(&sf.atomicSuccessfulWrites)

		// Add this storage folder to the list of storage folders.
		smfs = append(smfs, sfm)
	}
	return smfs
}

/*
// RemoveStorageFolder removes a storage folder from the host.
func (sm *StorageManager) RemoveStorageFolder(removalIndex int, force bool) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.resourceLock.RLock()
	defer sm.resourceLock.RUnlock()
	if sm.closed {
		return errStorageManagerClosed
	}

	// Check that the removal folder exists, and create a shortcut to it.
	if removalIndex >= len(sm.storageFolders) || removalIndex < 0 {
		return errBadStorageFolderIndex
	}
	removalFolder := sm.storageFolders[removalIndex]

	// Move all of the sectors in the storage folder to other storage folders.
	usedSize := removalFolder.Size - removalFolder.SizeRemaining
	offloadErr := sm.offloadStorageFolder(removalFolder, usedSize)
	// If 'force' is set, we want to ignore 'errIncopmleteOffload' and try to
	// remove the storage folder anyway. For any other error, we want to halt
	// and return the error.
	if force && offloadErr == errIncompleteOffload {
		offloadErr = nil
	}
	if offloadErr != nil {
		return offloadErr
	}

	// Remove the storage folder from the host and then save the host.
	sm.storageFolders = append(sm.storageFolders[0:removalIndex], sm.storageFolders[removalIndex+1:]...)
	removeErr := sm.dependencies.removeFile(filepath.Join(sm.persistDir, removalFolder.uidString()))
	saveErr := sm.saveSync()
	return composeErrors(saveErr, removeErr)
}

// ResizeStorageFolder changes the amount of disk space that is going to be
// allocated to a storage folder.
func (sm *StorageManager) ResizeStorageFolder(storageFolderIndex int, newSize uint64) error {
	// Lock the host for the duration of the resize operation - it is important
	// that the host not be manipulated while sectors are being moved around.
	sm.mu.Lock()
	defer sm.mu.Unlock()
	// The resource lock is required as the sector movements require access to
	// the logger.
	sm.resourceLock.RLock()
	defer sm.resourceLock.RUnlock()
	if sm.closed {
		return errStorageManagerClosed
	}

	// Check that the inputs are valid.
	if storageFolderIndex >= len(sm.storageFolders) || storageFolderIndex < 0 {
		return errBadStorageFolderIndex
	}
	resizeFolder := sm.storageFolders[storageFolderIndex]
	if newSize > maximumStorageFolderSize {
		return errLargeStorageFolder
	}
	if newSize < minimumStorageFolderSize {
		return errSmallStorageFolder
	}
	if resizeFolder.Size == newSize {
		return errNoResize
	}

	// Sectors do not need to be moved onto or away from the resize folder if
	// the folder is growing, or if after being shrunk the folder still has
	// enough storage to house all of the sectors it currently tracks.
	resizeFolderSizeConsumed := resizeFolder.Size - resizeFolder.SizeRemaining
	if resizeFolderSizeConsumed <= newSize {
		resizeFolder.SizeRemaining = newSize - resizeFolderSizeConsumed
		resizeFolder.Size = newSize
		return sm.saveSync()
	}

	// Calculate the number of sectors that need to be offloaded from the
	// storage folder.
	offloadSize := resizeFolderSizeConsumed - newSize
	offloadErr := sm.offloadStorageFolder(resizeFolder, offloadSize)
	if offloadErr == errIncompleteOffload {
		// Offloading has not fully succeeded, but may have partially
		// succeeded. To prevent new sectors from being added to the storage
		// folder, clamp the size of the storage folder to the current amount
		// of storage in use.
		resizeFolder.Size -= resizeFolder.SizeRemaining
		resizeFolder.SizeRemaining = 0
		return offloadErr
	} else if offloadErr != nil {
		return offloadErr
	}
	resizeFolder.Size = newSize
	resizeFolder.SizeRemaining = 0
	return sm.saveSync()
}
*/
