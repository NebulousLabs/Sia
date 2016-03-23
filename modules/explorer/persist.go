package explorer

import (
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/persist"

	"github.com/NebulousLabs/bolt"
)

var explorerMetadata = persist.Metadata{
	Header:  "Sia Explorer",
	Version: "0.5.2",
}

// initPersist initializes the persistent structures of the explorer module.
func (e *Explorer) initPersist() error {
	// Make the persist directory
	err := os.MkdirAll(e.persistDir, 0700)
	if err != nil {
		return err
	}

	// Open the database
	db, err := persist.OpenDatabase(explorerMetadata, filepath.Join(e.persistDir, "explorer.db"))
	if err != nil {
		return err
	}
	e.db = db

	// Initialize the database
	err = e.db.Update(func(tx *bolt.Tx) error {
		buckets := [][]byte{
			bucketBlockFacts,
			bucketBlockHashes,
			bucketBlocksDifficulty,
			bucketBlockTargets,
			bucketFileContractHistories,
			bucketFileContractIDs,
			bucketRecentChange,
			bucketSiacoinOutputIDs,
			bucketSiacoinOutputs,
			bucketSiafundOutputIDs,
			bucketSiafundOutputs,
			bucketTransactionHashes,
			bucketUnlockHashes,
		}
		for _, b := range buckets {
			_, err := tx.CreateBucketIfNotExists(b)
			if err != nil {
				return err
			}
		}
		return nil
	})

	return nil
}
