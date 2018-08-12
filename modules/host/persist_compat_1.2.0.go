package host

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/coreos/bbolt"

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
	v112StorageManagerDBFilename      = "storagemanager.db"
	v112StorageManagerDir             = "storagemanager"
	v112StorageManagerPersistFilename = "storagemanager.json"
)

var (
	// minimumStorageFolderSize specifies the minimum storage folder size
	// accepted by the new contract manager.
	//
	// NOTE: This number needs to be kept in sync with the actual minimum
	// storage folder size of the contract manager, but it is unlikely that
	// synchronization would be lost.
	minimumStorageFolderSize = contractManagerStorageFolderGranularity * modules.SectorSize

	// v112PersistMetadata is the header of the v112 host persist file.
	v112PersistMetadata = persist.Metadata{
		Header:  "Sia Host",
		Version: "0.5",
	}

	// v112StorageManagerBucketSectorUsage is the name of the bucket that
	// contains all of the sector usage information in the v1.0.0 storage
	// manager.
	v112StorageManagerBucketSectorUsage = []byte("BucketSectorUsage")

	// v112StorageManagerDBMetadata contains the legacy metadata for the v1.0.0
	// storage manager database. The version is v0.6.0, as that is the last
	// time that compatibility was broken with the storage manager persist.
	v112StorageManagerDBMetadata = persist.Metadata{
		Header:  "Sia Storage Manager DB",
		Version: "0.6.0",
	}

	// v112StorageManagerMetadata contains the legacy metadata for the v1.0.0
	// storage manager persistence. The version is v0.6.0, as that is the last time
	// that compatibility was broken with the storage manager persist.
	v112StorageManagerMetadata = persist.Metadata{
		Header:  "Sia Storage Manager",
		Version: "0.6.0",
	}
)

type (
	// v112StorageManagerPersist contains the legacy fields necessary to load the
	// v1.0.0 storage manager persistence.
	v112StorageManagerPersist struct {
		SectorSalt     crypto.Hash
		StorageFolders []*v112StorageManagerStorageFolder
	}

	// v112StorageManagerSector defines a sector held by the v1.0.0 storage
	// manager, which includes the data itself as well as all of the associated
	// metadata.
	v112StorageManagerSector struct {
		Count int
		Data  []byte
		Key   []byte
		Root  crypto.Hash
	}

	// v112StorageManagerSectorUsage defines the sectorUsage struct for the
	// v1.0.0 storage manager, the data loaded from the sector database.
	v112StorageManagerSectorUsage struct {
		Corrupted     bool
		Expiry        []types.BlockHeight
		StorageFolder []byte
	}

	// v112StorageManagerStorageFolder contains the legacy fields necessary to load
	// the v1.0.0 storage manager persistence.
	v112StorageManagerStorageFolder struct {
		Path          string
		Size          uint64
		SizeRemaining uint64
		UID           []byte
	}
)

// loadCompatV100 loads fields that have changed names or otherwise broken
// compatibility with previous versions, enabling users to upgrade without
// unexpected loss of data.
//
// COMPAT v1.0.0
//
// A spelling error in pre-1.0 versions means that, if this is the first time
// running after an upgrade, the misspelled field needs to be transferred over.
func (h *Host) loadCompatV100(p *persistence) error {
	var compatPersistence struct {
		FinancialMetrics struct {
			PotentialStorageRevenue types.Currency `json:"potentialerevenue"`
		}
		Settings struct {
			MinContractPrice          types.Currency `json:"contractprice"`
			MinDownloadBandwidthPrice types.Currency `json:"minimumdownloadbandwidthprice"`
			MinStoragePrice           types.Currency `json:"storageprice"`
			MinUploadBandwidthPrice   types.Currency `json:"minimumuploadbandwidthprice"`
		}
	}
	err := h.dependencies.LoadFile(v112PersistMetadata, &compatPersistence, filepath.Join(h.persistDir, settingsFile))
	if err != nil {
		return err
	}
	// Load the compat values, but only if the compat values are non-zero and
	// the real values are zero.
	if !compatPersistence.FinancialMetrics.PotentialStorageRevenue.IsZero() && p.FinancialMetrics.PotentialStorageRevenue.IsZero() {
		h.financialMetrics.PotentialStorageRevenue = compatPersistence.FinancialMetrics.PotentialStorageRevenue
	}
	if !compatPersistence.Settings.MinContractPrice.IsZero() && p.Settings.MinContractPrice.IsZero() {
		h.settings.MinContractPrice = compatPersistence.Settings.MinContractPrice
	}
	if !compatPersistence.Settings.MinDownloadBandwidthPrice.IsZero() && p.Settings.MinDownloadBandwidthPrice.IsZero() {
		h.settings.MinDownloadBandwidthPrice = compatPersistence.Settings.MinDownloadBandwidthPrice
	}
	if !compatPersistence.Settings.MinStoragePrice.IsZero() && p.Settings.MinStoragePrice.IsZero() {
		h.settings.MinStoragePrice = compatPersistence.Settings.MinStoragePrice
	}
	if !compatPersistence.Settings.MinUploadBandwidthPrice.IsZero() && p.Settings.MinUploadBandwidthPrice.IsZero() {
		h.settings.MinUploadBandwidthPrice = compatPersistence.Settings.MinUploadBandwidthPrice
	}
	return nil
}

// readAndDeleteV112Sectors reads some sectors from the v1.0.0 storage
// manager, deleting them from disk and returning. This clears up disk space
// for the new contract manager, though puts the data at risk of loss in the
// event of a power interruption. Risk window is small, amount of data at risk
// is small, so this is acceptable.
func (h *Host) readAndDeleteV112Sectors(oldPersist *v112StorageManagerPersist, oldDB *persist.BoltDatabase, numToFetch int) (sectors []v112StorageManagerSector, err error) {
	err = oldDB.Update(func(tx *bolt.Tx) error {
		// Read at most contractManagerStorageFolderGranularity sectors per
		// storage folder.
		sectorsPerStorageFolder := make(map[string]int)

		bucket := tx.Bucket(v112StorageManagerBucketSectorUsage)
		i := 0
		c := bucket.Cursor()
		for sectorKey, sectorUsageBytes := c.First(); sectorUsageBytes != nil && i < numToFetch; sectorKey, sectorUsageBytes = c.Next() {
			var usage v112StorageManagerSectorUsage
			err := json.Unmarshal(sectorUsageBytes, &usage)
			if err != nil {
				continue
			}

			// Don't read more than contractManagerStorageFolderGranularity
			// sectors per storage folder.
			readSoFar := sectorsPerStorageFolder[string(usage.StorageFolder)]
			if readSoFar >= contractManagerStorageFolderGranularity {
				continue
			}
			sectorsPerStorageFolder[string(usage.StorageFolder)]++

			// Read the sector from disk.
			sectorFilename := filepath.Join(h.persistDir, v112StorageManagerDir, hex.EncodeToString(usage.StorageFolder), string(sectorKey))
			sectorData, err := ioutil.ReadFile(sectorFilename)
			if err != nil {
				h.log.Println("Unable to read a sector from the legacy storage manager during host upgrade:", err)
			}

			// Delete the sector from disk.
			err = os.Remove(sectorFilename)
			if err != nil {
				h.log.Println("unable to remove sector from the legacy storage manager, be sure to remove manually:", err)
			}

			sector := v112StorageManagerSector{
				Count: len(usage.Expiry),
				Data:  sectorData,
				Key:   sectorKey,
				Root:  crypto.MerkleRoot(sectorData),
			}
			sectors = append(sectors, sector)
			i++
		}

		// Delete the usage data from the storage manager db for each of the
		// sectors.
		for _, sector := range sectors {
			err := bucket.Delete(sector.Key)
			if err != nil {
				h.log.Println("Unable to delete a sector from the bucket, the sector could not be found:", err)
			}
		}
		return nil
	})
	return sectors, err
}

// upgradeFromV112toV120 is an upgrade layer that migrates the host from
// the old storage manager to the new contract manager. This particular upgrade
// only handles migrating the sectors.
func (h *Host) upgradeFromV112ToV120() error {
	h.log.Println("Attempting an upgrade for the host from v1.0.0 to v1.2.0")

	// Sanity check - the upgrade will not work if the contract manager has not
	// been loaded yet.
	if h.StorageManager == nil {
		return errors.New("cannot perform host upgrade - the contract manager must not be nil")
	}

	// Fetch the old set of storage folders, and create analogous storage
	// folders in the contract manager. But create them to have sizes of zero,
	// and grow them 112 sectors at a time. This is to make sure the user does
	// not run out of disk space during the upgrade.
	oldPersist := new(v112StorageManagerPersist)
	err := persist.LoadJSON(v112StorageManagerMetadata, oldPersist, filepath.Join(h.persistDir, v112StorageManagerDir, v112StorageManagerPersistFilename))
	if err != nil {
		return build.ExtendErr("unable to load the legacy storage manager persist", err)
	}

	// Open the old storagemanager database.
	oldDB, err := persist.OpenDatabase(v112StorageManagerDBMetadata, filepath.Join(h.persistDir, v112StorageManagerDir, v112StorageManagerDBFilename))
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
		if !exists {
			newFoldersNeeded++
		}
	}

	// Pre-emptively read some sectors from the storage manager. This will
	// clear up space on disk to make room for the contract manager folders.
	//
	// NOTE: The sectorData returned for the sectors may be 'nil' if there
	// were disk I/O errors.
	sectors, err := h.readAndDeleteV112Sectors(oldPersist, oldDB, contractManagerStorageFolderGranularity*newFoldersNeeded)
	if err != nil {
		h.log.Println("Error reading sectors from legacy storage manager:", err)
	}

	// Iterate through each storage folder and create analogous storage folders
	// in the new contract manager. These storage folders may already exist in
	// the new contract manager.
	for _, sf := range oldPersist.StorageFolders {
		// Nothing to do if the contract manager already has this storage
		// folder (unusually situation though).
		_, exists := currentPaths[sf.Path]
		if exists {
			continue
		}

		// Create a storage folder in the contract manager for the
		// corresponding storage folder in the storage manager.
		err := h.AddStorageFolder(sf.Path, minimumStorageFolderSize)
		if err != nil {
			h.log.Println("Unable to create a storage folder in the contract manager:", err)
			continue
		}
	}

	// Add all of the preloaded sectors to the contract manager.
	var wg sync.WaitGroup
	for _, sector := range sectors {
		for i := 0; i < sector.Count; i++ {
			if uint64(len(sector.Data)) == modules.SectorSize {
				wg.Add(1)
				go func(sector v112StorageManagerSector) {
					err := h.AddSector(sector.Root, sector.Data)
					if err != nil {
						err = build.ExtendErr("Unable to add legacy sector to the upgraded contract manager:", err)
						h.log.Println(err)
					}
					wg.Done()
				}(sector)
			}
		}
	}
	wg.Wait()

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
		sectors, err := h.readAndDeleteV112Sectors(oldPersist, oldDB, contractManagerStorageFolderGranularity*canGrow)
		if err != nil {
			h.log.Println("Error reading sectors from legacy storage manager:", err)
			continue
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
					continue
				}
			}
		}

		// Add the sectors to the contract manager.
		var wg sync.WaitGroup
		for _, sector := range sectors {
			for i := 0; i < sector.Count; i++ {
				if uint64(len(sector.Data)) == modules.SectorSize {
					wg.Add(1)
					go func(sector v112StorageManagerSector) {
						err := h.AddSector(sector.Root, sector.Data)
						if err != nil {
							err = build.ExtendErr("Unable to add legacy sector to the upgraded contract manager:", err)
							h.log.Println(err)
						}
						wg.Done()
					}(sector)
				}
			}
		}
		wg.Wait()
	}

	// Save the desired storage folder sizes before closing out the old persist.
	cmFolders := h.StorageFolders()

	// Clean up up the old storage manager before growing the storage folders.
	// An interruption during the growing phase should result in storage folders
	// that are whatever size they were left off at.
	err = oldDB.Close()
	if err != nil {
		h.log.Println("Unable to close old database during v1.2.0 compat upgrade", err)
	}
	// Try loading the persist again.
	p := new(persistence)
	err = h.dependencies.LoadFile(v112PersistMetadata, p, filepath.Join(h.persistDir, settingsFile))
	if err != nil {
		return build.ExtendErr("upgrade appears complete, but having difficulties reloading host after upgrade", err)
	}
	h.loadPersistObject(p)
	// Apply the v100 compat upgrade in case the host is loading from a
	// version between v1.0.0 and v1.1.2.
	err = h.loadCompatV100(p)
	if err != nil {
		return build.ExtendErr("upgrade appears complete, but having trouble reloading:", err)
	}
	// Save the updated persist so that the upgrade is not triggered again.
	err = h.saveSync()
	if err != nil {
		return build.ExtendErr("upgrade appears complete, but final save has failed (upgrade likely successful", err)
	}
	// Delete the storage manager files. Note that this must happen after the
	// complete upgrade, including a finishing call to saveSync().
	for _, sf := range oldPersist.StorageFolders {
		err = os.Remove(filepath.Join(h.persistDir, v112StorageManagerDir, hex.EncodeToString(sf.UID)))
		if err != nil {
			h.log.Println("Unable to remove legacy contract manager files:", err)
		}
	}
	err = os.Remove(filepath.Join(h.persistDir, v112StorageManagerDir, v112StorageManagerPersistFilename))
	if err != nil {
		h.log.Println("Unable to remove legacy persist files:", err)
	}
	oldDB.Close()
	err = os.Remove(filepath.Join(h.persistDir, v112StorageManagerDir, v112StorageManagerDBFilename))
	if err != nil {
		h.log.Println("Unable to close legacy database:", err)
	}

	// Resize any remaining folders to their full size.
	for _, cmFolder := range cmFolders {
		finalCapacity := smFolderCapacities[cmFolder.Path]
		finalCapacity -= finalCapacity % (modules.SectorSize * contractManagerStorageFolderGranularity)
		if cmFolder.Capacity < finalCapacity {
			err := h.ResizeStorageFolder(cmFolder.Index, finalCapacity, false)
			if err != nil {
				err = build.ExtendErr("unable to resize storage folder during host upgrade", err)
				h.log.Println(err)
				continue
			}
		}
	}
	return nil
}
