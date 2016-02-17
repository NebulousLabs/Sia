package host

// sectors.go is responsible for mananging sectors within the host. The host
// outsources a lot of the management load to the filesystem by making each
// sector a different file, where the filename is the Merkle root of the
// sector. Multiple folder locations are supported, and sectors are sent to
// each disk sector through a process of consistent hashing.

// TODO: Make sure all the persist is moving over. In particular, the sector
// salt is important. Also, the sector salt needs to be documented.

// TODO: Perhaps instead of doing os.Rename, it makes sense to utilize
// 'addSector' and 'removeSector' - by doing that, you actually save rewriting
// code.

// TODO: Sector keys should be 12 bytes in the database. Actually, in general
// sectors should have 12 byte identifiers and they should be saved on disk as
// base64 to minimize the number of characters in the file name, to keep the
// filesystem load as light as possible.

// TODO: Storage folders must not conflict with eachother. Instead of having 32
// byte random names, they can just have 4 byte random names and then not
// conflict at all. To preserve cryptographic integrity with the hashing, a
// master key of 32 bytes can be kept which gets hashed against everything
// else. Oh wait, that's not needed anymore because it's optimal-placement.
// right.

// TODO: Cap the number of repeat sectors to something reasonable, like 10e3.

// TODO: Test simulating a disk failure, see what the host does. Hopefully,
// will still serve all the files it has and will not crash or malignantly
// handle any of the files it does not have.

// TODO: During renew, a host will grant a discount because some of the storage
// is repeated. But the host needs to know during an edit that it's not
// incurring extra cost. The host needs to know the end time of each sector, so
// that when a sector is edited it can tell how much money the renter owes. If
// a piece is being edited but does not affect the delete date of the sector,
// the edit must be paid for in full. But if the edit removes the need for the
// old piece entirely, only bandwidth and copy costs need to be paid, because
// no additional storage is actually being added. An extra level of trickiness
// occurs because the renter needs some way to konw if its getting all of the
// discounts that it is expecting. The renter can track 'optimal' and 'distance
// from optimal', and rate a host by how closely they are sticking to their
// advertised price in the optimal case. Hosts giving 80% discounts, etc. on
// moving things around will factor into their 'optimal distance' ratings,
// which will influence their weight for renewal.
//
// In sum, hosts need only track how much extra storage is being added during
// an edit and then can propose the fee for that operation. But now we have a
// problem with DoS vectors where hosts can reuse sectors a limited number of
// times, becuase the header object for the sector now has a size that grows
// linearly with the number of times that sector is used by various file
// contracts. Renters control the id of sectors, and therefore can manipulate
// the host to end up with large sectors. My solution is to limit sector reuse
// to 100x, beyond which the host will reject the sector and force the user to
// use some type of encryption.

// TODO: Need to add some command to 'siad' that will correctly repoint a
// storage folder to a new mountpoint. As best I can tell, this needs to happen
// while siad is not running. Either that, or 'siac' needs to do the whole
// shutdown thing itself? Still unclear.

// TODO: All modules should support load-backup-restore-close. And then there's
// this more subtle thing about persisting and staying in touch with the
// consensus set.

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

	// errInsufficientRemainingStorageForRemoval is returned if the remaining
	// storage folders do not have enough space remaining to support being
	// removed.
	errInsufficientRemainingStorageForRemoval = errors.New("not enough storage remaining to support removal of disk")

	// errInsufficientRemainingStorageForShrink is returned if the remaining
	// storage folders do not have enough space remaining to support being
	// reduced in size.
	errInsufficientRemainingStorageForShrink = errors.New("not enough storage remaining to support shrinking of disk")

	// errInsufficientStorageForSector is returned if the host tries to add a
	// sector when there is not enough storage remaining on the host to accept
	// the sector.
	//
	// Ideally, the host will adjust pricing as the host starts to fill up, so
	// this error should be pretty rare. Demand should drive the price up
	// faster than the Host runs out of space, such that the host is always
	// hovering around 95% capacity and rarely over 98% or under 90% capacity.
	errInsufficientStorageForSector = errors.New("not enough storage remaining to accept sector")

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

// storageFolder tracks the size and id of a folder that is being used to store
// sectors.
//
// Each storage folder has a short UID that is used for indexing. A simple
// counter did not seem sufficient because storage folders can be added and
// removed at random, meaning a counter would not necessarily match the order
// in which folders appear. The UID must be short because each entry in the
// sector usage database points to the storage folder that holds the sector.
// There are lots of sectors, which means a large name would blow up the size
// of the sector usage database.
//
// Because of the lookup, the random name, and the short name, there must be
// logic to check that two storage folders are not generated with the same
// name. This complicates testing, because 4 bytes is enough to make it
// unfeasible to test, yet 1 byte does not seem reasonable for a production
// value. Therefore, the size of the UID is different depending on the build.
type storageFolder struct {
	Size          uint64
	SizeRemaining uint64
	UID           []byte
}

// uidString returns the string value of the storage folder's UID, 8 characters
// of hex that represent the 4 byte UID.
func (sf *storageFolder) uidString() string {
	if len(sf.UID) != storageFolderUIDSize {
		build.Critical("sector UID is incorrect")
	}
	return hex.EncodeToString(sf.UID)
}

// emptiestStorageFolder returns the storage folder that has the lowest
// utilization.
func (h *Host) emptiestStorageFolder() *storageFolder {
	if len(h.storageFolders) == 0 {
		build.Critical("emptiest storage folder called when there are no storage folders at all")
	}

	lowestUtilization := float64(1)
	winningIndex := 0
	for i, sf := range h.storageFolders {
		sfUtilization := float64(sf.Size-sf.SizeRemaining) / float64(sf.Size)
		if sfUtilization < lowestUtilization {
			lowestUtilization = sfUtilization
			winningIndex = i
		}
	}
	return h.storageFolders[winningIndex]
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
	if size < minimumStorageFolderSize {
		return errSmallStorageFolder
	}
	// TODO: Add a check for breaking the maximum storage folder size.

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
			if bytes.Compare(newSF.UID, sf.UID) == 0 {
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

// RemoveStorageFolder removes a storage folder from the host.
func (h *Host) RemoveStorageFolder(removalIndex int) error {
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

	// Check that the removal folder exists.
	if removalIndex >= len(h.storageFolders) || removalIndex < 0 {
		return errBadStorageFolderIndex
	}

	// Create a shortcut variable for the storage folder being removed.
	removalFolder := h.storageFolders[removalIndex]

	// Check that there's enough room in the remaining disks to accept all of
	// the data being moved off of this disk.
	_, remainingStorage, err := h.capacity()
	if err != nil {
		return err
	}
	if remainingStorage < removalFolder.Size {
		return errInsufficientRemainingStorageForRemoval
	}

	// Create a new set of storage folders with the axed storage folder
	// removed. This action must be performed before the sector moving loop
	// begins, because the call to 'add sector' in the sector moving loop needs
	// to see the updated storage folders.
	oldStorageFolders := h.storageFolders
	if removalIndex == len(h.storageFolders)-1 {
		h.storageFolders = h.storageFolders[0:removalIndex]
	} else {
		h.storageFolders = append(h.storageFolders[0:removalIndex], h.storageFolders[removalIndex+1:]...)
	}

	// First read is to get the size of the bucket, and the value of the
	// first key. We are going to iterate through the bucket one key at a
	// time and do the move operation one sector at a time. This is slow,
	// but it means that if there is an error at any point, the host memory
	// and the database will still be consistent.
	var currentSectorID []byte
	var currentSectorBytes []byte
	err = h.db.View(func(tx *bolt.Tx) error {
		currentSectorID, currentSectorBytes = tx.Bucket(bucketSectorUsage).Cursor().First()
		return nil
	})
	if err != nil {
		// Need to reset the storage folders, because the removal has
		// failed.
		h.storageFolders = oldStorageFolders
		return err
	}

	// Go through the sectors one at a time until all sectors have been
	// reached.
	for currentSectorID != nil {
		err = h.db.Update(func(tx *bolt.Tx) error {
			// Determine whether the sector needs to be moved.
			var usage sectorUsage
			err = json.Unmarshal(currentSectorBytes, &usage)
			if err != nil {
				return err
			}
			if bytes.Compare(usage.StorageFolder, removalFolder.UID) != 0 {
				// Move on to the next bucket.
				bsuc := tx.Bucket(bucketSectorUsage).Cursor()
				bsuc.Seek(currentSectorID)
				currentSectorID, currentSectorBytes = bsuc.Next()
				// Returning nil will advance to the next iteration of the
				// loop.
				return nil
			}

			// This sector is in the removal folder, and therefore needs to
			// be moved to the next folder.
			nextFolder := h.emptiestStorageFolder()
			oldSectorPath := filepath.Join(h.persistDir, removalFolder.uidString(), string(currentSectorID))
			newSectorPath := filepath.Join(h.persistDir, nextFolder.uidString(), string(currentSectorID))
			err := os.Rename(oldSectorPath, newSectorPath)
			if err != nil {
				return err
			}
			removalFolder.SizeRemaining += sectorSize // for a later sanity check
			nextFolder.SizeRemaining -= sectorSize
			bsuc := tx.Bucket(bucketSectorUsage).Cursor()
			bsuc.Seek(currentSectorID)
			currentSectorID, currentSectorBytes = bsuc.Next()
			return nil
		})
		if err != nil {
			// Need to reset the storage folders, because the removal has
			// failed.
			h.storageFolders = oldStorageFolders
			return err
		}
	}
	// Sanity check - the removal folder should have no used space.
	if removalFolder.Size != removalFolder.SizeRemaining {
		build.Critical("storage folder removal process demonstrates divergence of storage tracking variables")
	}

	// Remove the symlink pointing to the old storage folder.
	err = os.Remove(filepath.Join(h.persistDir, removalFolder.uidString()))
	if err != nil {
		return err
	}
	return h.save()
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

	// Complain if an invalid sector index is provided.
	if storageFolderIndex >= len(h.storageFolders) || storageFolderIndex < 0 {
		return errBadStorageFolderIndex
	}
	resizeFolder := h.storageFolders[storageFolderIndex]
	if newSize < minimumStorageFolderSize {
		return errSmallStorageFolder
	}
	// TODO: Check the maximum size.
	if resizeFolder.Size == newSize {
		return errNoResize
	}

	// Sectors do not need to be moved onto or away from the resize folder if
	// the folder is growing, or if after being shrunk the folder still has
	// enough storage to house all of the sectors it currently tracks.
	oldSize := resizeFolder.Size
	resizeFolderSizeConsumed := resizeFolder.Size - resizeFolder.SizeRemaining
	if resizeFolder.Size < newSize || resizeFolderSizeConsumed <= newSize {
		resizeFolder.SizeRemaining = newSize - resizeFolderSizeConsumed
		resizeFolder.Size = newSize
		return h.save()
	}

	// Hitting this part of the function means that the folder is being reduced
	// in size, and that sectors need to be moved around for the shrink to be
	// successful.
	//
	// Determine how much storage needs to be reallocated, then figure out if
	// the remaining storage folders have enough space for it.
	_, remainingSpace, err := h.capacity()
	if err != nil {
		return err
	}
	externalSpaceNeeded := resizeFolderSizeConsumed - newSize
	externalSpaceAvailable := remainingSpace - resizeFolder.SizeRemaining
	if externalSpaceAvailable < externalSpaceNeeded {
		return errInsufficientRemainingStorageForShrink
	}

	// First read is to get the size of the bucket, and the value of the
	// first key. We are going to iterate through the bucket one key at a
	// time and do the move operation one sector at a time. This is slow,
	// but it means that if there is an error at any point, the host memory
	// and the database will still be consistent.
	var currentSectorID []byte
	var currentSectorBytes []byte
	err = h.db.View(func(tx *bolt.Tx) error {
		currentSectorID, currentSectorBytes = tx.Bucket(bucketSectorUsage).Cursor().First()
		return nil
	})
	if err != nil {
		// Need to reset the storage folders, because the removal has
		// failed.
		resizeFolder.Size = oldSize
		return err
	}

	// Go through the sectors one at a time until all sectors have been
	// reached.
	for currentSectorID != nil && resizeFolderSizeConsumed > 0 {
		err = h.db.Update(func(tx *bolt.Tx) error {
			// Determine whether the sector needs to be moved.
			var usage sectorUsage
			err = json.Unmarshal(currentSectorBytes, &usage)
			if err != nil {
				return err
			}
			if bytes.Compare(usage.StorageFolder, resizeFolder.UID) != 0 {
				// Move on to the next bucket.
				bsuc := tx.Bucket(bucketSectorUsage).Cursor()
				bsuc.Seek(currentSectorID)
				currentSectorID, currentSectorBytes = bsuc.Next()
				// Returning nil will advance to the next iteration of the
				// loop.
				return nil
			}

			// This sector is in the removal folder, and therefore needs to
			// be moved to the next folder.
			nextFolder := h.emptiestStorageFolder()
			oldSectorPath := filepath.Join(h.persistDir, resizeFolder.uidString(), string(currentSectorID))
			newSectorPath := filepath.Join(h.persistDir, nextFolder.uidString(), string(currentSectorID))
			err := os.Rename(oldSectorPath, newSectorPath)
			if err != nil {
				return err
			}
			resizeFolder.SizeRemaining -= sectorSize
			nextFolder.SizeRemaining += sectorSize
			bsuc := tx.Bucket(bucketSectorUsage).Cursor()
			bsuc.Seek(currentSectorID)
			currentSectorID, currentSectorBytes = bsuc.Next()
			resizeFolderSizeConsumed -= sectorSize // The iteration will stop when enough sectors have been moved away from the folder.
			return nil
		})
		if err != nil {
			// Need to reset the storage folders, because the removal has
			// failed.
			resizeFolder.Size = oldSize
			return err
		}
	}

	return h.save()
}
