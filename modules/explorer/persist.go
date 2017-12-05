package explorer

import (
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

const (
	explorerPersist = modules.ExplorerDir + ".json"
)

var explorerMetadata = persist.Metadata{
	Header:  "Sia Explorer",
	Version: "0.5.2",
}

type peristence struct {
	RecentChange modules.ConsensusChangeID
	Height       types.BlockHeight
	Target       types.Target
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
			bucketBlockIDs,
			bucketBlocksDifficulty,
			bucketBlockTargets,
			bucketFileContractHistories,
			bucketFileContractIDs,
			bucketInternal,
			bucketSiacoinOutputIDs,
			bucketSiacoinOutputs,
			bucketSiafundOutputIDs,
			bucketSiafundOutputs,
			bucketTransactionIDs,
			bucketUnlockHashes,
			bucketHashType,
		}
		for _, b := range buckets {
			_, err := tx.CreateBucketIfNotExists(b)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	filename := filepath.Join(e.persistDir, explorerPersist)
	_, err = os.Stat(filename)
	if os.IsNotExist(err) {
		return e.saveSync()
	} else if err != nil {
		return err
	}

	return e.load()
}

// load loads the explorer persistence from disk.
func (e *Explorer) load() error {
	return persist.LoadJSON(explorerMetadata, &e.persist, filepath.Join(e.persistDir, explorerPersist))
}

// saveSync saves the explorer persistence to disk, and then syncs to disk.
func (e *Explorer) saveSync() error {
	return persist.SaveJSON(explorerMetadata, e.persist, filepath.Join(e.persistDir, explorerPersist))
}
