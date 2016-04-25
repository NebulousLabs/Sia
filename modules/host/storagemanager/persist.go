package storagemanager

import (
	"crypto/rand"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/persist"

	"github.com/NebulousLabs/bolt"
)

// persistence is the data from the storage manager that gets saved to disk.
type persistence struct {
	SectorSalt     crypto.Hash
	StorageFolders []*storageFolder
}

// establishDefaults configures the default settings for the storage manager,
// overwriting any existing settings.
func (sm *StorageManager) establishDefaults() error {
	_, err := rand.Read(sm.sectorSalt[:])
	return err
}

// initDB will check that the database has been initialized and if not, will
// initialize the database.
func (sm *StorageManager) initDB() error {
	return sm.db.Update(func(tx *bolt.Tx) error {
		// The storage obligation bucket does not exist, which means the
		// database needs to be initialized. Create the database buckets.
		buckets := [][]byte{
			bucketSectorUsage,
		}
		for _, bucket := range buckets {
			_, err := tx.CreateBucketIfNotExists(bucket)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// load extracts the saved data from disk and applies it to the storage
// manager.
func (sm *StorageManager) load() error {
	p := new(persistence)
	err := sm.dependencies.loadFile(persistMetadata, p, filepath.Join(sm.persistDir, settingsFile))
	if os.IsNotExist(err) {
		// There is no host.json file, set up sane defaults.
		return sm.establishDefaults()
	} else if err != nil {
		return err
	}

	sm.sectorSalt = p.SectorSalt
	sm.storageFolders = p.StorageFolders
	return nil
}

// save stores all of the persistent data of the storage manager to disk.
func (sm *StorageManager) save(fsync bool) error {
	p := persistence{
		SectorSalt:     sm.sectorSalt,
		StorageFolders: sm.storageFolders,
	}
	return persist.SaveFile(persistMetadata, p, filepath.Join(sm.persistDir, settingsFile), fsync)
}
