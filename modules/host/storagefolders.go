package host

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

// TODO: Satisfy the the modules.Host interface.

// TODO: All exported functions should wrap the errors that they return so that
// there is more context for the user + developer when something goes wrong.

// TODO: There needs to be some way for ferrying persistent/ongoing errors to
// the user, like an error channel of some sort. I guess a log, but with more
// than just debugging information. Should Sia have a global log that is used
// to ferry important information to the user?

// TODO: Make sure all the persist is moving over. In particular, the sector
// salt is important. Also, the sector salt needs to be documented.

// TODO: Need to add some command to 'siad' that will correctly repoint a
// storage folder to a new mountpoint. As best I can tell, this needs to happen
// while siad is not running. Either that, or 'siac' needs to do the whole
// shutdown thing itself? Still unclear.

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/build"

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
	errLargeStorageFolder = fmt.Errorf("maximum allowed size for a storage folder is %v", maximumStorageFolderSize)

	// errMaxStorageFolders indicates that the limit on the number of allowed
	// storage folders has been reached.
	errMaxStorageFolders = fmt.Errorf("host can only accept up to %v storage folders", maximumStorageFolders)

	// errNoResize is returned if a new size is provided for a storage folder
	// that is the same as the current size of the storage folder.
	errNoResize = errors.New("storage folder selected for resize, but new size is same as current size")

	// errSmallStorageFolder is returned if a new storage folder is not large
	// enough to meet the requirements for the minimum storage folder size.
	errSmallStorageFolder = fmt.Errorf("minimum allowed size for a storage folder is %v", minimumStorageFolderSize)

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
type storageFolder struct {
	UID []byte

	Size          uint64
	SizeRemaining uint64

	FailedReads      uint64
	FailedWrites     uint64
	SuccessfulReads  uint64
	SuccessfulWrites uint64
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

// emptiestStorageFolder takes a set of storage folders and returns the storage
// folder with the lowest utilization by percentage. 'nil' is returned if there
// are no storage folders provided with sufficient free space for a sector.
//
// Refusing to return a storage folder that does not have enough space prevents
// the host from overfilling a storage folder.
func emptiestStorageFolder(sfs []*storageFolder) (*storageFolder, int) {
	lowestUtilization := float64(2) // Set higher than the max of 1 to protect against floating point imprecision.
	winningIndex := -1              // Set to impossible value to prevent unintentionally returning the wrong storage folder.
	winner := false
	for i, sf := range sfs {
		// Check that this storage folder has at least enough space to hold a
		// new sector. Also perform a sanity check that the storage folder has
		// a sane amount of storage remaining.
		if sf.SizeRemaining < sectorSize || sf.Size < sf.SizeRemaining {
			continue
		}
		winner = true // at least one storage folder has enough space for a new sector.

		// Check this storage folder against the current winning storage folder's utilization.
		sfUtilization := float64(sf.Size-sf.SizeRemaining) / float64(sf.Size)
		if sfUtilization < lowestUtilization {
			lowestUtilization = sfUtilization
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

// AddStorageFolder adds a storage folder to the host.
func (h *Host) AddStorageFolder(path string, size uint64) error {
	// Lock the host for the duration of the add operation - it is important
	// that the host not be manipulated while sectors are being moved around.
	h.mu.Lock()
	defer h.mu.Unlock()
	// The resource lock is required as the sector movements require access to
	// the logger.
	h.resourceLock.RLock()
	defer h.resourceLock.RUnlock()
	if h.closed {
		return errHostClosed
	}

	// Check that the maximum number of allowed storage folders has not been
	// exceeded.
	if len(h.storageFolders) >= maximumStorageFolders {
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
		Size:          size,
		SizeRemaining: size,
	}
	// Give the storage folder a new UID, while enforcing that the storage
	// folder can't have a collision with any of the other storage folders.
	newSF.UID = make([]byte, storageFolderUIDSize)
	for {
		// Generate an attempt UID for the storage folder.
		_, err = rand.Read(newSF.UID)
		if err != nil {
			return err
		}

		// Check for collsions. Check should be relatively inexpensive at all
		// times, because the total number of storage folders is limited to
		// 256.
		safe := true
		for _, sf := range h.storageFolders {
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
	symPath := filepath.Join(h.persistDir, newSF.uidString())
	err = os.Symlink(path, symPath)
	if err != nil {
		return err
	}

	// Add the storage folder to the list of folders for the host.
	h.storageFolders = append(h.storageFolders, newSF)
	return h.save()
}

// offloadStorageFolder takes sectors in a storage folder and moves them to
// another storage folder.
func (h *Host) offloadStorageFolder(offloadFolder *storageFolder, dataToOffload uint64) error {
	// The host is going to check every sector, using a different database tx
	// for each sector. To be able to track progress, a starting point needs to
	// be grabbed. This read grabs the starting point.
	var currentSectorID []byte
	var currentSectorBytes []byte
	err := h.db.View(func(tx *bolt.Tx) error {
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
	for _, sf := range h.storageFolders {
		if sf == offloadFolder {
			// The offload folder is not an available folder.
			continue
		}
		if sf.SizeRemaining < sectorSize {
			// Folders that don't have enough room for a new sector are not
			// available.
			continue
		}
		availableFolders = append(availableFolders, sf)
	}

	// Go through the sectors one at a time. Sectors that are not a part of the
	// provided storage folder are ignored. Sectors that are a part of the
	// storage folder will be moved to a new storage folder. The loop will
	// quick after 'dataToOffload' data has been moved from the storage folder.
	dataOffloaded := uint64(0)
	for currentSectorID != nil && dataOffloaded < dataToOffload && len(availableFolders) > 0 {
		err = h.db.Update(func(tx *bolt.Tx) error {
			// Determine whether the sector needs to be moved.
			var usage sectorUsage
			err = json.Unmarshal(currentSectorBytes, &usage)
			if err != nil {
				return err
			}
			if !bytes.Equal(usage.StorageFolder, offloadFolder.UID) {
				// The current sector is not in the offloading storage folder,
				// try the next sector.
				bsuc := tx.Bucket(bucketSectorUsage).Cursor()
				bsuc.Seek(currentSectorID)
				currentSectorID, currentSectorBytes = bsuc.Next()
				// Returning nil will advance to the next iteration of the
				// loop.
				return nil
			}

			// This sector is in the removal folder, and therefore needs to
			// be moved to the next folder.
			potentialFolders := availableFolders
			emptiestFolder, emptiestIndex := emptiestStorageFolder(potentialFolders)
			if emptiestFolder == nil {
				// If 'emptiestFolder' is nil, there are no storage folders
				// remaining that have enough storage to take on a new sector.
				// There is no point iterating through more sectors. Returning
				// nil will break out of this iteration, and setting available
				// folders to 'nil' will terminate the loop.
				availableFolders = nil
				return nil
			}

			success := false
			for emptiestFolder != nil {
				oldSectorPath := filepath.Join(h.persistDir, offloadFolder.uidString(), string(currentSectorID))
				// Try reading the sector from disk.
				sectorData, err := h.dependencies.ReadFile(oldSectorPath)
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

				newSectorPath := filepath.Join(h.persistDir, emptiestFolder.uidString(), string(currentSectorID))
				err = h.dependencies.WriteFile(newSectorPath, sectorData, 0700)
				if err != nil {
					// Indicate that the storage folder is having write
					// troubles.
					emptiestFolder.FailedWrites++

					// Because the write failed, we should move on to the next
					// storage folder and try that.
					potentialFolders = append(potentialFolders[0:emptiestIndex], potentialFolders[emptiestIndex+1:]...)

					// Try the next folder.
					emptiestFolder, emptiestIndex = emptiestStorageFolder(potentialFolders)
					continue
				}
				// Indicate that the storage folder is doing successful writes.
				emptiestFolder.SuccessfulWrites++
				err = h.dependencies.Remove(oldSectorPath)
				if err != nil {
					// Indicate that the storage folder is having write
					// troubles.
					offloadFolder.FailedWrites++
				}
				offloadFolder.SuccessfulWrites++

				success = true
				break
			}
			if !success {
				// The sector failed to be written successfully, try moving to
				// the next sector.
				return nil
			}

			offloadFolder.SizeRemaining += sectorSize
			emptiestFolder.SizeRemaining -= sectorSize
			dataOffloaded += sectorSize

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
			bsuc := tx.Bucket(bucketSectorUsage).Cursor()
			bsuc.Seek(currentSectorID)
			currentSectorID, currentSectorBytes = bsuc.Next()
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

// RemoveStorageFolder removes a storage folder from the host.
func (h *Host) RemoveStorageFolder(removalIndex int, force bool) error {
	// Lock the host for the duration of the remove operation - it is important
	// that the host not be manipulated while sectors are being moved around.
	h.mu.Lock()
	defer h.mu.Unlock()
	// The resource lock is required as the sector movements require access to
	// the logger.
	h.resourceLock.RLock()
	defer h.resourceLock.RUnlock()
	if h.closed {
		return errHostClosed
	}

	// Check that the removal folder exists, and create a shortcut to it.
	if removalIndex >= len(h.storageFolders) || removalIndex < 0 {
		return errBadStorageFolderIndex
	}
	removalFolder := h.storageFolders[removalIndex]

	// Move all of the sectors in the storage folder to other storage folders.
	usedSize := removalFolder.Size - removalFolder.SizeRemaining
	offloadErr := h.offloadStorageFolder(removalFolder, usedSize)
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
	oldStorageFolders := h.storageFolders
	h.storageFolders = append(h.storageFolders[0:removalIndex], h.storageFolders[removalIndex+1:]...)
	err := h.save()
	if err != nil {
		// Revert the storage folders to the old storage folders, since the
		// save operation has failed. Cap the size of the removalFolder so that
		// future negotiations will not try to use it to store sectors.
		//
		// This is done because a future save may succeed, but the host needs
		// to be kept consistent.
		h.storageFolders = oldStorageFolders
		removalFolder.Size = removalFolder.SizeRemaining
		removalFolder.SizeRemaining = 0

		// Check if the offload error needs to be composed into the save error.
		if offloadErr != nil {
			// There was an error in the offloading process, and that needs to
			// be composed into the return value.
			return errors.New(offloadErr.Error() + " and " + err.Error())
		}
		return err
	}

	// Remove the symlink that points to the data folder.
	err = h.dependencies.Remove(filepath.Join(h.persistDir, removalFolder.uidString()))
	if err != nil && offloadErr != nil {
		// Need to compose the offload error with the remove error.
		return errors.New(offloadErr.Error() + " and " + err.Error())
	} else if err != nil {
		return err
	}
	return offloadErr
}

// ResizeStorageFolder changes the amount of disk space that is going to be
// allocated to a storage folder.
func (h *Host) ResizeStorageFolder(storageFolderIndex int, newSize uint64) error {
	// Lock the host for the duration of the resize operation - it is important
	// that the host not be manipulated while sectors are being moved around.
	h.mu.Lock()
	defer h.mu.Unlock()
	// The resource lock is required as the sector movements require access to
	// the logger.
	h.resourceLock.RLock()
	defer h.resourceLock.RUnlock()
	if h.closed {
		return errHostClosed
	}

	// Check that the inputs are valid.
	if storageFolderIndex >= len(h.storageFolders) || storageFolderIndex < 0 {
		return errBadStorageFolderIndex
	}
	resizeFolder := h.storageFolders[storageFolderIndex]
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
		return h.save()
	}

	// Calculate the number of sectors that need to be offloaded from the
	// storage folder.
	offloadSize := resizeFolderSizeConsumed - newSize
	offloadErr := h.offloadStorageFolder(resizeFolder, offloadSize)
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
	return h.save()
}
