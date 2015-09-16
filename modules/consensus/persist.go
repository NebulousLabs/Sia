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
	return cs.db.Update(func(tx *bolt.Tx) error {
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
		return nil
	})
}

// loadDiffs is a transitional function to load the processed blocks
// from disk and move the diffs into memory
func (cs *ConsensusSet) loadDiffs() {
	height := cs.db.pathHeight()

	// Load all blocks from disk.
	for i := types.BlockHeight(0); i < height; i++ {
		bid := cs.db.getPath(i)
		pb := cs.db.getBlockMap(bid)

		lockID := cs.mu.Lock()
		cs.updateSubscribers(nil, []*processedBlock{pb})
		cs.mu.Unlock(lockID)
	}
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

	// Send any blocks that were loaded from disk to subscribers.
	cs.loadDiffs()

	return nil
}
