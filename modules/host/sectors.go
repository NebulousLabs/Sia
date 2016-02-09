package host

// sectors.go is responsible for mananging sectors within the host. The host
// outsources a lot of the management load to the filesystem by making each
// sector a different file, where the filename is the Merkle root of the
// sector. Multiple folder locations are supported, and sectors are sent to
// each disk sector through a process of consistent hashing.

// TODO: Test simulating a disk failure, see what the host does. Hopefully,
// will still serve all the files it has and will not crash or malignantly
// handle any of the files it does not have.

import (
	"crypto/rand"
	"math/big"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

var (
	// ErrBadStorageFolderIndex is returned if a storage folder is requested
	// that does not have the correct index.
	ErrBadStorageFolderIndex = errors.New("no storage folder exists at that index")

	// ErrInsufficientRemainingStorageForRemoval is returned if the remaining storage folders do not have enough space remaining to support 
	ErrInsufficientRemainingStorageForRemoval = errors.New("not enough storage remaining to support removal of disk")
)

// storageFolder tracks the size and id of a folder that is being used to store
// sectors.
type storageFolder struct {
	Size uint64
	UID  crypto.Hash
}

// addStorageFolder adds a storage folder to the host.
func (h *Host) addStorageFolder(path string, size uint64) error {
	// Create a storage folder object.
	newSF := storageFolder {
		Size: size,
	}
	// Give the storage folder a new UID.
	_, err := rand.Read(newSF.UID[:])
	if err != nil {
		return err
	}

	// Symlink the path for the data to the UID location of the host.
	symPath := filepath.Join(h.persistDir, string(newSF.UID[:]))
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
			// Try all storage folders except the current one, find the max.
			var greatestScore *big.Int
			var greatestSF int
			for i, sf := range h.storageFolders {
				score := types.Target(crypto.HashAll(sf.UID, key)).Int()
				score = score.Mul(score, big.NewInt(int64(sf.Size)))
				if score.Cmp(greatestScore) > 0 {
					greatestScore = score
					greatestSF = i
				}
			}

			// Determine if this sector should be moved from its current
			// location to the new location.
			newSFScore := types.Target(crypto.HashAll(newSF.UID, key)).Int()
			newSFScore = newSFScore.Mul(newSFScore, big.NewInt(int64(size)))
			if newSFScore.Cmp(greatestScore) > 0 {
				// The sector should be moved to the new location.
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

// removeStorageFolder removes a storage folder from the host.
func (h *Host) removeStorageFolder(removalIndex int) error {
	// Check that the storage folder being deleted exists.
	if removalIndex >= len(h.storageFolders) {
		return ErrBadStorageFolderIndex
	}

	// Check that there's enough room in the remaining disks to accept all of
	// the data being moved off of this disk - to account for the turmoil,
	// there should be about 2% extra room after this disk is removed.
	var totalStorage uint64
	for i, sf := range h.storageFolders {
		if i == removalIndex {
			continue
		}
		totalStorage += sf.Size
	}
	if h.spaceRemaining - h.storageFolders[removalIndex].Size < totalStorage / 50 {
		return ErrInsuffiicentRemainingStorage
	}

	// Create a new set of storage folders with the axed storage folder
	// removed.

	newSFs := append(
	// Open up the database of sectors and score them against the folders to
	// figure out where they currently exist, and where they belong.
	err = h.db.View(func(tx *bolt.Tx) error {
		bsu := tx.Bucket(BucketSectorUsage)
		bsuc := bsu.Cursor()
		for key, _ := bsuc.First(); key != nil; key, _ = bsuc.Next() {
			// Try all storage folders except the current one, find the max.
			var greatestScore *big.Int
			var greatestSF int
			for i, sf := range h.storageFolders {
				score := types.Target(crypto.HashAll(sf.UID, key)).Int()
				score = score.Mul(score, big.NewInt(int64(sf.Size)))
				if score.Cmp(greatestScore) > 0 {
					greatestScore = score
					greatestSF = i
				}
			}

			// Determine if this sector should be moved from its current
			// location to a different sector.
			if greatestSF == index {
			}
			newSFScore := types.Target(crypto.HashAll(newSF.UID, key)).Int()
			newSFScore = newSFScore.Mul(newSFScore, big.NewInt(int64(size)))
			if newSFScore.Cmp(greatestScore) > 0 {
				// The sector should be moved to the new location.
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
}

// resizeStorageFolder changes the amount of disk space that is going to be
// allocated to a storage folder.
func (h *Host) resizeStorageFolder(index int, newSize uint64) {
}

func (h *Host) addSector() {
}

func (h *Host) removeSector() {
}

// Sector update code - for use when adding sectors to the host.
/*
	bsu := tx.Bucket(BucketSectorUsage)
	for _, root := range so.SectorRoots {
		// Check if there is already a sector with this data.
		sectorUsageBytes := bsu.Get(root[:])
		if sectorUsageBytes != nil {
			// This sector is already in use. Decode the number of times it
			// is in use, increment the counter, and then store the usage
			// information.
			usage := binary.BigEndian.Uint64(sectorUsageBytes)
			binary.BigEndian.PutUint64(sectorUsageBytes, usage+1)
			err = bsu.Put(sectorUsageBytes)
			if err != nil {
				return err
			}
		} else {
			// This sector is not in use yet. Encode '1' to indicate that
			// the sector is in use in one time.
			sectorUsageBytes = make([]byte, 8)
			binary.BigEndian.PutUint64(sectorUsageBytes, 1)
			err = bsu.Put(sectorUsageBytes)
			if err != nil {
				return err
			}
		}
	}
*/
