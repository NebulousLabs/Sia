package consensus

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/boltdb/bolt"

	"github.com/NebulousLabs/Sia/types"
)

const (
	DatabaseFilename = "consensus.db"
)

// loadDB pulls all the blocks that have been saved to disk into memory, using
// them to fill out the ConsensusSet.
func (cs *ConsensusSet) loadDB() error {
	// Open the database - a new bolt database will be created if none exists.
	db, err := openDB(filepath.Join(cs.persistDir, DatabaseFilename))
	if err != nil {
		return err
	}
	cs.db = db

	// Walk through initialization for Sia.
	var height types.BlockHeight
	err = cs.db.Update(func(tx *bolt.Tx) error {
		// Check if the database has been initialized.
		if !dbInitialized(tx) {
			return cs.initDB(tx)
		}

		// Check that the genesis block is correct - typically only incorrect
		// in the event of developer binaries vs. release binaires.
		genesisID := getPath(tx, 0)
		if genesisID != cs.blockRoot.Block.ID() {
			return errors.New("Blockchain has wrong genesis block, exiting.")
		}

		height = blockHeight(tx)
		return nil
	})
	if err != nil {
		return err
	}

	// Send all of the existing blocks to subscribers - temporary while
	// subscribers don't have any persistence for block progress.
	err = cs.db.View(func(tx *bolt.Tx) error {
		for i := types.BlockHeight(0); i <= height; i++ {
			// Fetch the processed block at height 'i'.
			id := getPath(tx, i)
			pb, err := getBlockMap(tx, id)
			if err != nil {
				return err
			}

			// Send the block to subscribers.
			cs.updateSubscribers(nil, []*processedBlock{pb})
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// initPersist initializes the persistence structures of the consensus set, in
// particular loading the database and preparing to manage subscribers.
func (cs *ConsensusSet) initPersist() error {
	// Create the consensus directory.
	err := os.MkdirAll(cs.persistDir, 0700)
	if err != nil {
		return err
	}

	// Try to load an existing database from disk - a new one will be created
	// if one does not exist.
	err = cs.loadDB()
	if err != nil {
		return err
	}

	return nil
}
