package host

// sectors.go is responsible for mananging sectors within the host. The host
// outsources a lot of the management load to the filesystem by making each
// sector a different file, where the filename is the Merkle root of the
// sector. Multiple folder locations are supported, and sectors are sent to
// each disk sector through a process of consistent hashing.
//
// Rendezvous hashing is used to determine which storage folder should be added
// to disk. Sectors that are reused will collide, saving the host disk space
// and allowing renters to do cheaper overlapping file contract renewals. A
// sector is scored by hashing its Merkle root against the UID of all storage
// folders. The score from the storage folder is then multiplied by the size of
// the storage folder. The storage folder with the largest score wins. Storage
// folders that are twice as large are twice as likely to win. When a storage
// folder is resized, only a fraction of the sectors will need to be moved
// around.

// TODO: Cap the number of storage folders to something reasonable, like 250.

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

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

var (
	// ErrBadStorageFolderIndex is returned if a storage folder is requested
	// that does not have the correct index.
	ErrBadStorageFolderIndex = errors.New("no storage folder exists at that index")

	// ErrInsufficientRemainingStorageForRemoval is returned if the remaining
	// storage folders do not have enough space remaining to support being
	// removed.
	ErrInsufficientRemainingStorageForRemoval = errors.New("not enough storage remaining to support removal of disk")

	// ErrInsufficientRemainingStorageForShrink is returned if the remaining
	// storage folders do not have enough space remaining to support being
	// reduced in size.
	ErrInsufficientRemainingStorageForShrink = errors.New("not enough storage remaining to support shrinking of disk")

	// ErrInsufficientStorageForSector is returned if the host tries to add a
	// sector when there is not enough storage remaining on the host to accept
	// the sector.
	//
	// Ideally, the host will adjust pricing as the host starts to fill up, so
	// this error should be pretty rare. Demand should drive the price up
	// faster than the Host runs out of space, such that the host is always
	// hovering around 95% capacity and rarely over 98% or under 90% capacity.
	ErrInsufficientStorageForSector = errors.New("not enough storage remaining to accept sector")

	// ErrNoResize is returned if a new size is provided for a storage folder
	// that is the same as the current size of the storage folder.
	ErrNoResize = errors.New("storage folder selected for resize, but new size is same as current size")

	// ErrSmallStorageFolder is returned if a new storage folder is not large
	// enough to meet the requirements for the minimum storage folder size.
	ErrSmallStorageFolder = fmt.Errorf("minimum allowed size for a storage folder is %v", minimumStorageFolderSize)

	// ErrStorageFolderNotFolder is returned if a storage folder gets added
	// that is not a folder.
	ErrStorageFolderNotFolder = errors.New("must use to an existing folder")
)

// storageFolder tracks the size and id of a folder that is being used to store
// sectors.
type storageFolder struct {
	Size uint64
	UID  crypto.Hash
}

// sectorUsage indicates how a sector is being used. Each block height
// represents a point at which a file contract using the sector expires. File
// contracts that use the sector multiple times will have their block height
// appear multiple times. This data allows the host to figure out what types of
// discounts can be applied to data that is reusing sectors. This is primarily
// useful for file contract renewals, and really shouldn't be used otherwise.
type sectorUsage struct {
	Expiry []types.BlockHeight
}

// greatestStorageFolder determines which storage folder has the greatest score
// for a given sector hash and returns the index of the greatest storage folder
// as well as the score of that folder.
//
// For convenience, the sector hash is accepted as a byte array.
func greatestStorageFolder(sectorRoot crypto.Hash, storageFolderSet []storageFolder) (winningIndex int, greatestScore types.Currency) {
	for i, sf := range storageFolderSet {
		score := types.NewCurrency(types.Target(crypto.HashAll(sf.UID, sectorRoot[:])).Int())
		score = score.Mul(types.NewCurrency64(sf.Size))
		if score.Cmp(greatestScore) > 0 {
			greatestScore = score
			winningIndex = i
		}
	}
	return winningIndex, greatestScore
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

	// Check that the storage folder being added meets the minimum requirement
	// for the size of a storage folder.
	if size < minimumStorageFolderSize {
		return ErrSmallStorageFolder
	}

	// Check that the folder being linked to both exists and is a folder.
	pathInfo, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !pathInfo.Mode().IsDir() {
		return ErrStorageFolderNotFolder
	}

	// Create a storage folder object.
	newSF := storageFolder{
		Size: size,
	}
	// Give the storage folder a new UID.
	_, err = rand.Read(newSF.UID[:])
	if err != nil {
		return err
	}

	// Symlink the path for the data to the UID location of the host.
	symPath := filepath.Join(h.persistDir, newSF.UID.String())
	err = os.Symlink(path, symPath)
	if err != nil {
		return err
	}

	// Open up the database of sectors and score them against the folders to
	// figure out where they currently exist, and where they belong.
	err = h.db.View(func(tx *bolt.Tx) error {
		bsu := tx.Bucket(BucketSectorUsage)
		bsuc := bsu.Cursor()
		for key, _ := bsuc.First(); key != nil; key, _ = bsuc.Next() {
			// Get the score of the greatest storage folder for this sector
			// hash.
			var sectorRoot crypto.Hash
			copy(sectorRoot[:], key)
			greatestSF, greatestScore := greatestStorageFolder(sectorRoot, h.storageFolders)

			// Determine if this sector should be moved from its current
			// location to the newly added storage folder.
			newSFScore := types.NewCurrency(types.Target(crypto.HashAll(newSF.UID, sectorRoot)).Int())
			newSFScore = newSFScore.Mul(types.NewCurrency64(size))
			if newSFScore.Cmp(greatestScore) > 0 {
				// The new storage folder scores higher for this sector, which
				// means that the sector should be moved.
				oldSectorPath := filepath.Join(h.persistDir, string(h.storageFolders[greatestSF].UID[:]))
				newSectorPath := filepath.Join(h.persistDir, string(newSF.UID[:]))
				err = os.Rename(oldSectorPath, newSectorPath)
				if err != nil {
					h.log.Println("ERROR: could not copy sector from", oldSectorPath, "to", newSectorPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		// Returning an error is the correct action. Even if there was a disk
		// failure partway through the copying process, trying again should be
		// able to correctly handle both trying to copy things that were
		// already copied and of copything over the sectors that had not yet
		// been copied.
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

	// Check that the storage folder being deleted exists.
	if removalIndex >= len(h.storageFolders) {
		return ErrBadStorageFolderIndex
	}

	// Check that there's enough room in the remaining disks to accept all of
	// the data being moved off of this disk - to account for the turmoil,
	// there should be about 2% extra room after this disk is removed.
	totalStorage, remainingStorage, err := h.capacity()
	if err != nil {
		return err
	}
	if remainingStorage-h.storageFolders[removalIndex].Size < totalStorage/50 {
		return ErrInsufficientRemainingStorageForRemoval
	}

	// Create a new set of storage folders with the axed storage folder
	// removed.
	var newStorageFolders []storageFolder
	if removalIndex == len(h.storageFolders)-1 {
		newStorageFolders = h.storageFolders[0:removalIndex]
	} else {
		newStorageFolders = append(h.storageFolders[0:removalIndex], h.storageFolders[removalIndex+1:]...)
	}

	// Open up the database of sectors and score them against the folders to
	// figure out where they currently exist, and where they belong.
	err = h.db.View(func(tx *bolt.Tx) error {
		bsu := tx.Bucket(BucketSectorUsage)
		bsuc := bsu.Cursor()
		for key, _ := bsuc.First(); key != nil; key, _ = bsuc.Next() {
			// Determine if this sector is in the storage folder that is being
			// removed.
			var sectorRoot crypto.Hash
			copy(sectorRoot[:], key)
			greatestSF, _ := greatestStorageFolder(sectorRoot, h.storageFolders)
			if greatestSF != removalIndex {
				// Skip this sector, as it is not in the storage folder that is
				// being removed.
				continue
			}

			// Determine which storage folder should receive the displaced
			// sector, then move the sector from its current storage folder to
			// the new greatest storage folder.
			newGreatestSF, _ := greatestStorageFolder(sectorRoot, newStorageFolders)
			oldSectorPath := filepath.Join(h.persistDir, string(h.storageFolders[removalIndex].UID[:]))
			newSectorPath := filepath.Join(h.persistDir, string(newStorageFolders[newGreatestSF].UID[:]))
			err := os.Rename(oldSectorPath, newSectorPath)
			if err != nil {
				h.log.Println("ERROR: could not copy sector from", oldSectorPath, "to", newSectorPath)
			}
		}
		return nil
	})
	if err != nil {
		// Returning an error is the correct action. Even if there was a disk
		// failure partway through the copying process, trying again should be
		// able to correctly handle both trying to copy things that were
		// already copied and of copything over the sectors that had not yet
		// been copied.
		return err
	}

	h.storageFolders = newStorageFolders
	return h.save()
}

// growStorageFolder will increase the size of a storage folder, appropriately
// moving around sectors as necessary.
func (h *Host) growStorageFolder(storageFolderIndex int, newSize uint64) error {
	// TODO: There are some sanity checks that happen in a parent function, but
	// maybe they should be repeated here for sanity's sake. Or maybe they
	// should only be performed in the child fucntion? I'm leaning towards only
	// having them performed in the child function.

	// Open up the database of sectors and score them against the folders to
	// figure out where they currently exist, and whether they now need to be
	// moved to the increased sector.
	err := h.db.View(func(tx *bolt.Tx) error {
		bsu := tx.Bucket(BucketSectorUsage)
		bsuc := bsu.Cursor()
		for key, _ := bsuc.First(); key != nil; key, _ = bsuc.Next() {
			// Find the greatest storage folder and score for this sector.
			var sectorRoot crypto.Hash
			copy(sectorRoot[:], key)
			greatestSF, greatestScore := greatestStorageFolder(sectorRoot, h.storageFolders)
			if greatestSF == storageFolderIndex {
				// Sector is already in the current storage folder, and the
				// current storage folder score can only increase, meaning that
				// the sector is guaranteed to not need to move.
				continue
			}

			// Determine if this sector should be moved from its current
			// storage folder to the storage folder that has been increased in
			// size.
			sfuid := h.storageFolders[storageFolderIndex].UID
			increasedSFScore := types.NewCurrency(types.Target(crypto.HashAll(sfuid, sectorRoot)).Int())
			increasedSFScore = increasedSFScore.Mul(types.NewCurrency64(newSize))
			if increasedSFScore.Cmp(greatestScore) > 0 {
				// The sector should be moved from its current storage folder
				// to the newly larger storage folder.
				oldSectorPath := filepath.Join(h.persistDir, string(h.storageFolders[greatestSF].UID[:]))
				newSectorPath := filepath.Join(h.persistDir, string(sfuid[:]))
				err := os.Rename(oldSectorPath, newSectorPath)
				if err != nil {
					h.log.Println("ERROR: could not copy sector from", oldSectorPath, "to", newSectorPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		// Returning an error is the correct action. Even if there was a disk
		// failure partway through the copying process, trying again should be
		// able to correctly handle both trying to copy things that were
		// already copied and of copything over the sectors that had not yet
		// been copied.
		return err
	}

	h.storageFolders[storageFolderIndex].Size = newSize
	return h.save()
}

// shrinkStorageFolder will decrease the size of a storage folder,
// appropriately moving around sectors as necessary.
func (h *Host) shrinkStorageFolder(storageFolderIndex int, newSize uint64) error {
	// TODO: There are some sanity checks that happen in a parent function, but
	// maybe they should be repeated here for sanity's sake. Or maybe they
	// should only be performed in the child fucntion? I'm leaning towards only
	// having them performed in the child function.

	// Check that there's enough room in the remaining disks to accept all of
	// the data being moved off of this disk - to account for the turmoil,
	// there should be about 2% extra room after this disk is removed.
	totalStorage, remainingStorage, err := h.capacity()
	if err != nil {
		return err
	}
	if remainingStorage-(h.storageFolders[storageFolderIndex].Size-newSize) < totalStorage/50 {
		return ErrInsufficientRemainingStorageForRemoval
	}

	// If this is the only storage folder, no sectors need to be moved around.
	if len(h.storageFolders) == 1 {
		h.storageFolders[storageFolderIndex].Size = newSize
		return h.save()
	}

	// Open up the database of sectors to figure out which sectors get
	// displaced by the shrink operation.
	err = h.db.View(func(tx *bolt.Tx) error {
		bsu := tx.Bucket(BucketSectorUsage)
		bsuc := bsu.Cursor()
		for key, _ := bsuc.First(); key != nil; key, _ = bsuc.Next() {
			// Determine if this sector is in the storage folder that is being
			// shrunk.
			var sectorRoot crypto.Hash
			copy(sectorRoot[:], key)
			greatestSF, _ := greatestStorageFolder(sectorRoot, h.storageFolders)
			if greatestSF != storageFolderIndex {
				// Sectors can only be removed from the shrinking storage
				// folder, so if the sector is not in the shrinking storage
				// folder then it does not need to be moved.
				continue
			}

			// Grab the second greatest storage folder, accounting for the
			// shrinking of the current storage folder. The greatest storage
			// folder after shrinking is determined by updating the host
			// storage folders as though the shrinking is complete, finding the
			// greatest storage folder among the updated set, and then
			// returning the host storage folders to their original state.
			oldSize := h.storageFolders[storageFolderIndex].Size
			h.storageFolders[storageFolderIndex].Size = newSize
			secondSF, _ := greatestStorageFolder(sectorRoot, h.storageFolders)
			h.storageFolders[storageFolderIndex].Size = oldSize
			if secondSF == storageFolderIndex {
				// If, after being shrunk, the shrinking storage folder is
				// still the greatest, no operation needs to be performed.
				continue
			}
			// The sector needs to be moved, because a different storage folder
			// has won the race.
			oldSectorPath := filepath.Join(h.persistDir, string(h.storageFolders[storageFolderIndex].UID[:]))
			newSectorPath := filepath.Join(h.persistDir, string(h.storageFolders[secondSF].UID[:]))
			err := os.Rename(oldSectorPath, newSectorPath)
			if err != nil {
				h.log.Println("ERROR: could not copy sector from", oldSectorPath, "to", newSectorPath)
			}
		}
		return nil
	})
	if err != nil {
		// Returning an error is the correct action. Even if there was a disk
		// failure partway through the copying process, trying again should be
		// able to correctly handle both trying to copy things that were
		// already copied and of copything over the sectors that had not yet
		// been copied.
		return err
	}
	h.storageFolders[storageFolderIndex].Size = newSize
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
		return ErrBadStorageFolderIndex
	}
	if newSize < minimumStorageFolderSize {
		return ErrSmallStorageFolder
	}
	if h.storageFolders[storageFolderIndex].Size == newSize {
		return ErrNoResize
	}

	// Different logic needs to be run depending on whether the storage folder
	// is being increased in size or is being decreased in size. Compare the
	// current size of the storage folder to the new size and run the
	// appropriate logic.
	if h.storageFolders[storageFolderIndex].Size > newSize {
		return h.growStorageFolder(storageFolderIndex, newSize)
	}
	return h.shrinkStorageFolder(storageFolderIndex, newSize)
}

// addSector will add a data sector to the host, correctly selecting the
// storage folder in which the sector belongs.
func (h *Host) addSector(sectorRoot crypto.Hash, expiryHeight types.BlockHeight, sectorData []byte) error {
	// Sanity check - sector should have sectorSize bytes.
	if uint64(len(sectorData)) != sectorSize {
		build.Critical("incorrectly sized sector passed to addSector in the host")
		return errors.New("incorrectly sized sector passed to addSector in the host")
	}
	// Expensive sanity check - the sector should have a root that matches the
	// sectorRoot.
	if build.DEBUG {
		verifiedRoot, err := crypto.ReaderMerkleRoot(bytes.NewReader(sectorData))
		if err != nil {
			build.Critical(err)
		}
		if verifiedRoot != sectorRoot {
			build.Critical("incorrectly hashed sector passed to the host")
		}
	}

	// Check that there is enough room for the sector.
	_, remainingStorage, err := h.capacity()
	if remainingStorage < sectorSize {
		return ErrInsufficientStorageForSector
	}

	// Determine which storage folder is going to receive the new sector.
	err = h.db.Update(func(tx *bolt.Tx) error {
		// Update the database to reflect the new sector.
		bsu := tx.Bucket(BucketSectorUsage)
		usageBytes := bsu.Get(sectorRoot[:])
		var usage sectorUsage
		if usageBytes != nil {
			// usageBytes is typically going to be nil, as sectors are unlikely
			// to already be in the usage database.
			err := json.Unmarshal(usageBytes, &usage)
			if err != nil {
				return err
			}
		}
		usage.Expiry = append(usage.Expiry, expiryHeight)
		usageBytes, err = json.Marshal(usage)
		if err != nil {
			return err
		}
		err = bsu.Put(sectorRoot[:], usageBytes)
		if err != nil {
			return err
		}

		greatestSF, _ := greatestStorageFolder(sectorRoot, h.storageFolders)
		sectorPath := filepath.Join(h.persistDir, h.storageFolders[greatestSF].UID.String(), sectorRoot.String())
		return ioutil.WriteFile(sectorPath, sectorData, 0700)
	})
	if err != nil {
		return err
	}
	return h.save()
}

// removeSector will remove a data sector from the host.
func (h *Host) removeSector(sectorRoot crypto.Hash, expiryHeight types.BlockHeight) error {
	// Determine which storage folder is going to receive the new sector.
	return h.db.Update(func(tx *bolt.Tx) error {
		// Update the database to reflect the new sector.
		bsu := tx.Bucket(BucketSectorUsage)
		usageBytes := bsu.Get(sectorRoot[:])
		var usage sectorUsage
		err := json.Unmarshal(usageBytes, &usage)
		if err != nil {
			return err
		}
		if len(usage.Expiry) > 1 {
			// Find any entry in the usage that's at the expiry height and
			// remove it.
			var i int
			for i := 0; i < len(usage.Expiry); i++ {
				if usage.Expiry[i] == expiryHeight {
					break
				}
			}
			if i == len(usage.Expiry) {
				return errors.New("removing a sector that doesn't seem to exist")
			}
			if i == len(usage.Expiry)-1 {
				usage.Expiry = append(usage.Expiry[0:i])
			} else {
				usage.Expiry = append(usage.Expiry[0:i], usage.Expiry[i+1:]...)
			}
			usageBytes, err = json.Marshal(usage)
			if err != nil {
				return err
			}
			return bsu.Put(sectorRoot[:], usageBytes)
		}
		// Delete the element of the bucket - it's now empty.
		err = bsu.Delete(sectorRoot[:])
		if err != nil {
			return err
		}

		// Figure out which storage folder is holding the sector.
		greatestSF, _ := greatestStorageFolder(sectorRoot, h.storageFolders)
		sectorPath := filepath.Join(h.persistDir, string(h.storageFolders[greatestSF].UID[:]))
		return os.Remove(sectorPath)
	})
}
