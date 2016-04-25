package storagemanager

// storgaefolder.go is responsible for managing the storage folders within the
// host. Storage folders can be added, resized, or removed. There are several
// features in place to make sure that the host is always using a reasonable
// amount of resources. Sectors in the host are currently always 4MiB, though
// support for different sizes is planned. Becaues of the reliance on the
// cached Merkle trees, sector sizes are likely to always be a power of 2.
//
// Though storage folders each contain a bunch of sectors, there is no mapping
// from a storage folder to the sectors that it contains. Instead, one must
// either look at the filesystem or go through the sector usage database.
// There is a mapping from a sector to the storage folder that it is in, so a
// list of sectors for each storage folder can be obtained, though the
// operation is expensive. It is not recommended that you try to look at the
// filesystem to see all of the sectors in a storage folder, because all of the
// golang implementations that let you do this load the whole directory into
// memory at once, and these directories may contain millions of sectors.
//
// Strict resource limits are maintained, to make sure that any user behavior
// which would strain the host will return an error instead of cause the user
// problems. The number of storage folders is capped, the allowed size for a
// storage folder has a range, and anything else that might have a linear or
// nonconstant effect on resource consumption is capped.
//
// Sectors are meant to be spread out across the storage folders as evenly as
// possible, but this is done in a very passive way. When a storage folder is
// added, sectors are not moved from the other storage folder to optimize for a
// quick operation. When a storage folder is reduced in size, sectors are only
// moved if there is not enough room on the remainder of the storage folder to
// hold all of the sectors.
//
// Storage folders are identified by an ID. This ID is short (4 bytes) and is
// randomly generated but is guaranteed not to conflict with any other storage
// folder IDs (if a conflict is generated randomly, a new random folder is
// chosen). A counter was rejected because storage folders can be removed and
// added arbitrarily, and there should be a firm difference between accessing a
// storage folder by index vs. accessing a storage folder by id.
//
// Storage folders statically track how much of their storage is unused.
// Because there is no mapping from a storage folder to the sectors that it
// contains, a static mapping must be manually maintained. While it would be
// possible to track which sectors are in each storage folder by using nested
// buckets in the sector usage database, the implementation cost is high, and
// is perceived to be higher than the implementation cost of statically
// tracking the amount of storage remaining. Also the introduction of nested
// buckets relies on fancier, less used features in the boltdb dependency,
// which carries a higher error risk.

// TODO: Need to add some command to 'siad' that will correctly repoint a
// storage folder to a new mountpoint. As best I can tell, this needs to happen
// while siad is not running. Either that, or 'siac' needs to do the whole
// shutdown thing itself? Still unclear.

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"

	"github.com/NebulousLabs/bolt"
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
	errLargeStorageFolder = fmt.Errorf("maximum allowed size for a storage folder is %v bytes", maximumStorageFolderSize)

	// errMaxStorageFolders indicates that the limit on the number of allowed
	// storage folders has been reached.
	errMaxStorageFolders = fmt.Errorf("host can only accept up to %v storage folders", maximumStorageFolders)

	// errNoResize is returned if a new size is provided for a storage folder
	// that is the same as the current size of the storage folder.
	errNoResize = errors.New("storage folder selected for resize, but new size is same as current size")

	// errSmallStorageFolder is returned if a new storage folder is not large
	// enough to meet the requirements for the minimum storage folder size.
	errSmallStorageFolder = fmt.Errorf("minimum allowed size for a storage folder is %v bytes", minimumStorageFolderSize)

	// errStorageFolderNotFolder is returned if a storage folder gets added
	// that is not a folder.
	errStorageFolderNotFolder = errors.New("must use to an existing folder")
)

// storageFolder tracks a folder that is being used to store sectors. There is
// a corresponding symlink in the host directory that points to whatever folder
// the user has chosen for storing data (usually, a separate drive will be
// mounted at that point).
//
// 'Size' is set by the user, indicating how much data can be placed into that
// folder before the host should consider it full. Size is measured in bytes,
// but only accounts for the actual raw data. Sia also places a nontrivial
// amount of load on the filesystem, potentially to the tune of millions of
// files. These files have long, cryptographic names and may take up as much as
// a gigabyte of space in filesystem overhead, depending on how the filesystem
// is architected. The host is programmed to gracefully handle full disks, so
// wihle it might cause the user surprise that the host can't break past 99%
// utilization, there should not be any issues if the user overestimates how
// much storage is available in the folder they have offered to Sia. The host
// will put the drive at 100% utilization, which may cause performance
// slowdowns or other errors if non-Sia programs are also trying to use the
// filesystem. If users are experiencing problems, having them set the storage
// folder size to 98% of the actual drive size is probably going to fix most of
// the issues.
//
// 'SizeRemaining' is a variable that remembers how much storage is remaining
// in the storage folder. It is managed manually, and is updated every time a
// sector is added to or removed from the storage folder. Because there is no
// property that inherently guarantees the correctness of 'SizeRemaining',
// implementation must be careful to maintain consistency.
//
// The UID of the storage folder is a small number of bytes that uniquely
// identify the storage folder. The UID is generated randomly, but in such a
// way as to guarantee that it will not collide with the ids of other storage
// folders. The UID is used (via the uidString function) to determine the name
// of the symlink which points to the folder holding the data for this storage
// folder.
//
// Statistics are kept on the integrity of reads and writes. Ideally, the
// filesystem is never returning errors, but if errors are being returned they
// will be tracked and can be reported to the user.
type storageFolder struct {
	Path string
	UID  []byte

	Size          uint64
	SizeRemaining uint64

	FailedReads      uint64
	FailedWrites     uint64
	SuccessfulReads  uint64
	SuccessfulWrites uint64
}

// emptiestStorageFolder takes a set of storage folders and returns the storage
// folder with the lowest utilization by percentage. 'nil' is returned if there
// are no storage folders provided with sufficient free space for a sector.
//
// Refusing to return a storage folder that does not have enough space prevents
// the host from overfilling a storage folder.
func emptiestStorageFolder(sfs []*storageFolder) (*storageFolder, int) {
	mostFree := float64(-1) // Set lower than the min amount available to protect from floating point imprecision.
	winningIndex := -1      // Set to impossible value to prevent unintentionally returning the wrong storage folder.
	winner := false
	for i, sf := range sfs {
		// Check that this storage folder has at least enough space to hold a
		// new sector. Also perform a sanity check that the storage folder has
		// a sane amount of storage remaining.
		if sf.SizeRemaining < modules.SectorSize || sf.Size < sf.SizeRemaining {
			continue
		}
		winner = true // at least one storage folder has enough space for a new sector.

		// Check this storage folder against the current winning storage folder's utilization.
		sfFree := float64(sf.SizeRemaining) / float64(sf.Size)
		if mostFree < sfFree {
			mostFree = sfFree
			winningIndex = i
		}
	}
	// Do not return any storage folder if none of them have enough room for a
	// new sector.
	if !winner {
		return nil, -1
	}
	return sfs[winningIndex], winningIndex
}

// offloadStorageFolder takes sectors in a storage folder and moves them to
// another storage folder.
func (sm *StorageManager) offloadStorageFolder(offloadFolder *storageFolder, dataToOffload uint64) error {
	// The host is going to check every sector, using a different database tx
	// for each sector. To be able to track progress, a starting point needs to
	// be grabbed. This read grabs the starting point.
	//
	// It is expected that the host is under lock for the whole operation -
	// this function should be the only function with access to the database.
	var currentSectorID []byte
	var currentSectorBytes []byte
	err := sm.db.View(func(tx *bolt.Tx) error {
		currentSectorID, currentSectorBytes = tx.Bucket(bucketSectorUsage).Cursor().First()
		return nil
	})
	if err != nil {
		return err
	}

	// Create a list of available folders. As folders are filled up, this list
	// will be pruned. Once all folders are full, the offload loop will quit
	// and return with errIncompleteOffload.
	availableFolders := make([]*storageFolder, 0)
	for _, sf := range sm.storageFolders {
		if sf == offloadFolder {
			// The offload folder is not an available folder.
			continue
		}
		if sf.SizeRemaining < modules.SectorSize {
			// Folders that don't have enough room for a new sector are not
			// available.
			continue
		}
		availableFolders = append(availableFolders, sf)
	}

	// Go through the sectors one at a time. Sectors that are not a part of the
	// provided storage folder are ignored. Sectors that are a part of the
	// storage folder will be moved to a new storage folder. The loop will quit
	// after 'dataToOffload' data has been moved from the storage folder.
	dataOffloaded := uint64(0)
	for currentSectorID != nil && dataOffloaded < dataToOffload && len(availableFolders) > 0 {
		err = sm.db.Update(func(tx *bolt.Tx) error {
			// Defer seeking to the next sector.
			defer func() {
				bsuc := tx.Bucket(bucketSectorUsage).Cursor()
				bsuc.Seek(currentSectorID)
				currentSectorID, currentSectorBytes = bsuc.Next()
			}()

			// Determine whether the sector needs to be moved.
			var usage sectorUsage
			err = json.Unmarshal(currentSectorBytes, &usage)
			if err != nil {
				return err
			}
			if !bytes.Equal(usage.StorageFolder, offloadFolder.UID) {
				// The current sector is not in the offloading storage folder,
				// try the next sector. Returning nil will advance to the next
				// iteration of the loop.
				return nil
			}

			// This sector is in the removal folder, and therefore needs to
			// be moved to the next folder.
			success := false
			emptiestFolder, emptiestIndex := emptiestStorageFolder(availableFolders)
			for emptiestFolder != nil {
				oldSectorPath := filepath.Join(sm.persistDir, offloadFolder.uidString(), string(currentSectorID))
				// Try reading the sector from disk.
				sectorData, err := sm.dependencies.readFile(oldSectorPath)
				if err != nil {
					// Inidicate that the storage folder is having read
					// troubles.
					offloadFolder.FailedReads++

					// Returning nil will move to the next sector. Though the
					// current sector has failed to read, the host will keep
					// trying future sectors in hopes of finishing the task.
					return nil
				}
				// Indicate that the storage folder did a successful read.
				offloadFolder.SuccessfulReads++

				// Try writing the sector to the emptiest storage folder.
				newSectorPath := filepath.Join(sm.persistDir, emptiestFolder.uidString(), string(currentSectorID))
				err = sm.dependencies.writeFile(newSectorPath, sectorData, 0700)
				if err != nil {
					// Indicate that the storage folder is having write
					// troubles.
					emptiestFolder.FailedWrites++

					// After the failed write, try removing any garbage that
					// may have gotten left behind. The error is not checked,
					// as it is known that the disk is having write troubles.
					_ = sm.dependencies.removeFile(newSectorPath)

					// Because the write failed, we should move on to the next
					// storage folder, and remove the current storage folder
					// from the list of available folders.
					availableFolders = append(availableFolders[0:emptiestIndex], availableFolders[emptiestIndex+1:]...)

					// Try the next folder.
					emptiestFolder, emptiestIndex = emptiestStorageFolder(availableFolders)
					continue
				}
				// Indicate that the storage folder is doing successful writes.
				emptiestFolder.SuccessfulWrites++
				err = sm.dependencies.removeFile(oldSectorPath)
				if err != nil {
					// Indicate that the storage folder is having write
					// troubles.
					offloadFolder.FailedWrites++
				} else {
					offloadFolder.SuccessfulWrites++
				}

				success = true
				break
			}
			if !success {
				// The sector failed to be written successfully, try moving to
				// the next sector.
				return nil
			}

			offloadFolder.SizeRemaining += modules.SectorSize
			emptiestFolder.SizeRemaining -= modules.SectorSize
			dataOffloaded += modules.SectorSize

			// Update the sector usage database to reflect the file movement.
			// Because this cannot be done atomically, recovery tools are
			// required to deal with outlier cases where the swap is fine but
			// the database update is not.
			usage.StorageFolder = emptiestFolder.UID
			newUsageBytes, err := json.Marshal(usage)
			if err != nil {
				return err
			}
			err = tx.Bucket(bucketSectorUsage).Put(currentSectorID, newUsageBytes)
			if err != nil {
				return err
			}

			// Seek to the next sector.
			return nil
		})
		if err != nil {
			return err
		}
	}
	if dataOffloaded < dataToOffload {
		return errIncompleteOffload
	}
	return nil
}

// storageFolder returns the storage folder in the host with the input uid. If
// the storage folder is not found, nil is returned.
func (sm *StorageManager) storageFolder(uid []byte) *storageFolder {
	for _, sf := range sm.storageFolders {
		if bytes.Equal(uid, sf.UID) {
			return sf
		}
	}
	return nil
}

// uidString returns the string value of the storage folder's UID. This string
// maps to the filename of the symlink that is used to point to the folder that
// holds all of the sector data contained by the storage folder.
func (sf *storageFolder) uidString() string {
	if len(sf.UID) != storageFolderUIDSize {
		build.Critical("sector UID length is incorrect - perhaps the wrong version of Sia is being run?")
	}
	return hex.EncodeToString(sf.UID)
}

// AddStorageFolder adds a storage folder to the host.
func (sm *StorageManager) AddStorageFolder(path string, size uint64) error {
	// Lock the host for the duration of the add operation - it is important
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

	// Check that the maximum number of allowed storage folders has not been
	// exceeded.
	if len(sm.storageFolders) >= maximumStorageFolders {
		return errMaxStorageFolders
	}

	// Check that the storage folder being added meets the minimum requirement
	// for the size of a storage folder.
	if size > maximumStorageFolderSize {
		return errLargeStorageFolder
	}
	if size < minimumStorageFolderSize {
		return errSmallStorageFolder
	}

	// Check that the folder being linked to both exists and is a folder.
	pathInfo, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !pathInfo.Mode().IsDir() {
		return errStorageFolderNotFolder
	}

	// Create a storage folder object.
	newSF := &storageFolder{
		Path: path,

		Size:          size,
		SizeRemaining: size,
	}
	// Give the storage folder a new UID, while enforcing that the storage
	// folder can't have a collision with any of the other storage folders.
	newSF.UID = make([]byte, storageFolderUIDSize)
	for {
		// Generate an attempt UID for the storage folder.
		_, err = sm.dependencies.randRead(newSF.UID)
		if err != nil {
			return err
		}

		// Check for collsions. Check should be relatively inexpensive at all
		// times, because the total number of storage folders is limited to
		// 256.
		safe := true
		for _, sf := range sm.storageFolders {
			if bytes.Equal(newSF.UID, sf.UID) {
				safe = false
				break
			}
		}
		if safe {
			break
		}
	}

	// Symlink the path for the data to the UID location of the host.
	symPath := filepath.Join(sm.persistDir, newSF.uidString())
	err = sm.dependencies.symlink(path, symPath)
	if err != nil {
		return err
	}

	// Add the storage folder to the list of folders for the host.
	sm.storageFolders = append(sm.storageFolders, newSF)
	return sm.save(true)
}

// ResetStorageFolderHealth will reset the read and write statistics for the
// storage folder.
func (sm *StorageManager) ResetStorageFolderHealth(index int) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.resourceLock.RLock()
	defer sm.resourceLock.RUnlock()
	if sm.closed {
		return errStorageManagerClosed
	}

	// Check that the input is valid.
	if index >= len(sm.storageFolders) {
		return errBadStorageFolderIndex
	}

	// Reset the storage statistics and save the host.
	sm.storageFolders[index].FailedReads = 0
	sm.storageFolders[index].FailedWrites = 0
	sm.storageFolders[index].SuccessfulReads = 0
	sm.storageFolders[index].SuccessfulWrites = 0
	return sm.save(false)
}

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
	saveErr := sm.save(true)
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
		return sm.save(true)
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
	return sm.save(true)
}

// StorageFolders provides information about all of the storage folders in the
// host.
func (sm *StorageManager) StorageFolders() (sfms []modules.StorageFolderMetadata) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, sf := range sm.storageFolders {
		sfms = append(sfms, modules.StorageFolderMetadata{
			Capacity:          sf.Size,
			CapacityRemaining: sf.SizeRemaining,
			Path:              sf.Path,

			FailedReads:      sf.FailedReads,
			FailedWrites:     sf.FailedWrites,
			SuccessfulReads:  sf.SuccessfulReads,
			SuccessfulWrites: sf.SuccessfulWrites,
		})
	}
	return sfms
}
