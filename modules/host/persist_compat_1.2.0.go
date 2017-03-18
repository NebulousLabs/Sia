package host

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/bolt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// contractManagerStorageFolderGranularity is a mirror of the storage
	// folder granularity constant in the contract manager. The two values need
	// to remain equal, however it is unlikely that it will ever change from
	// 64.
	contractManagerStorageFolderGranularity = 64

	// The directory names and filenames of legacy storage manager files.
	v100StorageManagerDir             = "storagemanager"
	v100StorageManagerDBFilename      = "storagemanager.db"
	v100StorageManagerPersistFilename = "storagemanager.json"
)

var (
	// minimumStorageFolderSize specifies the minimum storage folder size
	// accepted by the new contract manager.
	//
	// NOTE: This number needs to be kept in sync with the actual minimum
	// storage folder size of the contract manager, but it is unlikely that
	// synchronization would be lost.
	minimumStorageFolderSize = contractManagerStorageFolderGranularity * modules.SectorSize

	// v100StorageManagerBucketSectorUsage is the name of the bucket that
	// contains all of the sector usage information in the v1.0.0 storage
	// manager.
	v100StorageManagerBucketSectorUsage = []byte("BucketSectorUsage")

	// v100StorageManagerDBMetadata contains the legacy metadata for the v1.0.0
	// storage manager database. The version is v0.6.0, as that is the last
	// time that compatibility was broken with the storage manager persist.
	v100StorageManagerDBMetadata = persist.Metadata{
		Header:  "Sia Storage Manager DB",
		Version: "0.6.0",
	}

	// v100StorageManagerMetadata contains the legacy metadata for the v1.0.0
	// storage manager persistence. The version is v0.6.0, as that is the last time
	// that compatibility was broken with the storage manager persist.
	v100StorageManagerMetadata = persist.Metadata{
		Header:  "Sia Storage Manager",
		Version: "0.6.0",
	}
)

type (
	// v100StorageManagerPersist contains the legacy fields necessary to load the
	// v1.0.0 storage manager persistence.
	v100StorageManagerPersist struct {
		SectorSalt     crypto.Hash
		StorageFolders []*v100StorageManagerStorageFolder
	}

	// v100StorageManagerSector defines a sector held by the v1.0.0 storage
	// manager, which includes the data itself as well as all of the associated
	// metadata.
	v100StorageManagerSector struct {
		Count int
		Data  []byte
		Root  crypto.Hash
	}

	// v100StorageManagerSectorUsage defines the sectorUsage struct for the
	// v1.0.0 storage manager, the data loaded from the sector database.
	v100StorageManagerSectorUsage struct {
		Corrupted     bool
		Expiry        []types.BlockHeight
		StorageFolder []byte
	}

	// v100StorageManagerStorageFolder contains the legacy fields necessary to load
	// the v1.0.0 storage manager persistence.
	v100StorageManagerStorageFolder struct {
		Path string
		Size uint64
		UID  []byte
	}
)

// v100StorageManagerSectorID transforms a sector id in to a sector key.
func (h *Host) v100StorageManagerSectorID(sectorSalt crypto.Hash, sectorRoot []byte) []byte {
	saltedRoot := crypto.HashAll(sectorRoot, sectorSalt)
	id := make([]byte, base64.RawURLEncoding.EncodedLen(12))
	base64.RawURLEncoding.Encode(id, saltedRoot[:12])
	return id
}

// readAndDeleteV100Sectors reads some sectors from the v1.0.0 storage
// manager, deleting them from disk and returning. This clears up disk space
// for the new contract manager, though puts the data at risk of loss in the
// event of a power interruption. Risk window is small, amount of data at risk
// is small, so this is acceptable.
func (h *Host) readAndDeleteV100Sectors(oldPersist *v100StorageManagerPersist, oldDB *persist.BoltDatabase, numToFetch int) (sectors []v100StorageManagerSector, err error) {
	err = oldDB.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(v100StorageManagerBucketSectorUsage)
		i := 0
		c := bucket.Cursor()
		for sectorRoot, sectorUsageBytes := c.First(); sectorUsageBytes != nil && i < numToFetch; sectorRoot, sectorUsageBytes = c.Next() {
			var usage v100StorageManagerSectorUsage
			err := json.Unmarshal(sectorUsageBytes, &usage)
			if err != nil {
				return err
			}

			// Read the sector from disk.
			sectorKey := h.v100StorageManagerSectorID(oldPersist.SectorSalt, sectorRoot)
			sectorFilename := filepath.Join(h.persistDir, v100StorageManagerDir, hex.EncodeToString(usage.StorageFolder), string(sectorKey))
			sectorData, err := ioutil.ReadFile(sectorFilename)
			if err != nil {
				h.log.Println("Unable to read a sector from the legacy storage manager during host upgrade:", err)
			}

			// Delete the sector from disk.
			err = os.Remove(sectorFilename)
			if err != nil {
				h.log.Println("unable to remove sector from the legacy storage manager, be sure to remove manually:", err)
			}

			sector := v100StorageManagerSector{
				Count: len(usage.Expiry),
				Data:  sectorData,
			}
			copy(sector.Root[:], sectorRoot)
			sectors = append(sectors, sector)
		}

		// Delete the usage data from the storage manager db for each of the
		// sectors.
		for _, sector := range sectors {
			bucket.Delete(sector.Root[:]) // Error is ignored.
		}
		return nil
	})
	return sectors, err
}

// upgradeFromV100toV120 is an upgrade layer that migrates the host from
// the old storage manager to the new contract manager. This particular upgrade
// only handles migrating the sectors.
func (h *Host) upgradeFromV100toV120() error {
	h.log.Println("Attempting an upgrade for the host from v1.0.0 to v1.2.0")

	// Sanity check - the upgrade will not work if the contract manager has not
	// been loaded yet.
	if h.StorageManager == nil {
		return errors.New("cannot perform host upgrade - the contract manager must not be nil")
	}

	// Fetch the old set of storage folders, and create analagous storage
	// folders in the contract manager. But create them to have sizes of zero,
	// and grow them 100 sectors at a time. This is to make sure the user does
	// not run out of disk space during the upgrade.
	oldPersist := new(v100StorageManagerPersist)
	err := persist.LoadFile(v100StorageManagerMetadata, oldPersist, filepath.Join(h.persistDir, v100StorageManagerDir, v100StorageManagerPersistFilename))
	if err != nil {
		return build.ExtendErr("unable to load the legacy storage manager persist", err)
	}

	// Open the old storagemanager database.
	oldDB, err := persist.OpenDatabase(v100StorageManagerDBMetadata, filepath.Join(h.persistDir, v100StorageManagerDir, v100StorageManagerDBFilename))
	if err != nil {
		return build.ExtendErr("unable to open the legacy storage manager database", err)
	}

	// Create a map from old storage folders to their capacity.
	smFolderCapacities := make(map[string]uint64)
	for _, smFolder := range oldPersist.StorageFolders {
		smFolderCapacities[smFolder.Path] = smFolder.Size
	}

	// Fetch the set of storage folders already in the current contract
	// manager. When replacing existing storage folders in the storage manager,
	// duplicates will be avoided. Duplicates would otherwise be likely in the
	// event of a power outage during the upgrade.
	currentPaths := make(map[string]struct{})
	currentStorageFolders := h.StorageFolders()
	for _, sf := range currentStorageFolders {
		currentPaths[sf.Path] = struct{}{}
	}

	// Count the number of storage folders that need to be created in the
	// contract manager.
	var newFoldersNeeded int
	for _, sf := range oldPersist.StorageFolders {
		_, exists := currentPaths[sf.Path]
		if exists {
			newFoldersNeeded++
		}
	}

	// Pre-emptively read some sectors from the storage manager. This will
	// clear up space on disk to make room for the contract manager folders.
	//
	// NOTE: The sectorData returned for the sectors may be 'nil' if there
	// were disk I/O errors.
	sectors, err := h.readAndDeleteV100Sectors(oldPersist, oldDB, contractManagerStorageFolderGranularity*newFoldersNeeded)
	if err != nil {
		h.log.Println("Error reading sectors from legacy storage manager:", err)
		return err
	}

	// Iterate through each storage folder and create analogous storage folders
	// in the new contract manager. These storage folders may already exist in
	// the new contract manager.
	for _, sf := range oldPersist.StorageFolders {
		// Nothing to do if the contract manager already has this storage
		// folder (unusualy situation though).
		_, exists := currentPaths[sf.Path]
		if !exists {
			continue
		}

		// Create a storage folder in the contract manager for the
		// corresponding storage folder in the storage manager.
		err := h.AddStorageFolder(sf.Path, minimumStorageFolderSize)
		if err != nil {
			h.log.Println("Unable to create a storage folder in the contract manager:", err)
			return err
		}
	}

	// Add all of the preloaded sectors to the contract manager.
	for _, sector := range sectors {
		for i := 0; i < sector.Count; i++ {
			if uint64(len(sector.Data)) == modules.SectorSize {
				err = h.AddSector(sector.Root, sector.Data)
				if err != nil {
					err = build.ExtendErr("Unable to add legacy sector to the upgraded contract manager:", err)
					h.log.Println(err)
					return err
				}
			}
		}
	}

	// Read sectors from the storage manager database until all of the sectors
	// have been read.
	for {
		// Determine whether any of the storage folders need to be grown.
		var canGrow int
		cmFolders := h.StorageFolders()
		for _, cmFolder := range cmFolders {
			finalCapacity := smFolderCapacities[cmFolder.Path]
			if cmFolder.Capacity < finalCapacity-(modules.SectorSize*contractManagerStorageFolderGranularity) {
				canGrow++
			}
		}

		// Read some sectors from the storage manager.
		//
		// NOTE: The sectorData returned for the sectors may be 'nil' if there
		// were disk I/O errors.
		sectors, err := h.readAndDeleteV100Sectors(oldPersist, oldDB, contractManagerStorageFolderGranularity*canGrow)
		if err != nil {
			h.log.Println("Error reading sectors from legacy storage manager:", err)
			return err
		}
		// Break condition - if no sectors were read, the migration is
		// complete.
		if len(sectors) == 0 {
			break
		}

		// Grow the storage folders that are able to be grown.
		for _, cmFolder := range cmFolders {
			finalCapacity := smFolderCapacities[cmFolder.Path]
			if cmFolder.Capacity < finalCapacity-(modules.SectorSize*contractManagerStorageFolderGranularity) {
				err := h.ResizeStorageFolder(cmFolder.Index, cmFolder.Capacity+(modules.SectorSize*contractManagerStorageFolderGranularity), false)
				if err != nil {
					err = build.ExtendErr("unable to resize storage folder during host upgrade:", err)
					h.log.Println(err)
					return err
				}
			}
		}

		// Add the sectors to the contract manager.
		for _, sector := range sectors {
			for i := 0; i < sector.Count; i++ {
				if uint64(len(sector.Data)) == modules.SectorSize {
					err = h.AddSector(sector.Root, sector.Data)
					if err != nil {
						err = build.ExtendErr("Unable to add legacy sector to the upgraded contract manager:", err)
						h.log.Println(err)
						return err
					}
				}
			}
		}
	}

	// Resize any remaining folders to their full size.
	cmFolders := h.StorageFolders()
	for _, cmFolder := range cmFolders {
		finalCapacity := smFolderCapacities[cmFolder.Path]
		finalCapacity -= finalCapacity % (modules.SectorSize * contractManagerStorageFolderGranularity)
		if cmFolder.Capacity < finalCapacity {
			err := h.ResizeStorageFolder(cmFolder.Index, finalCapacity, false)
			if err != nil {
				err = build.ExtendErr("unable to resize storage folder during host upgrade:", err)
				h.log.Println(err)
				return err
			}
		}
	}

	// Clean up by deleting all the previous storage manager files.
	err = os.RemoveAll(filepath.Join(h.persistDir, v100StorageManagerDir))
	if err != nil {
		return build.ExtendErr("unable to remove the legacy storage manager folder", err)
	}
	return nil
}
