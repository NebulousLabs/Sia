package consensus

import (
	"os"
	"path/filepath"

	"github.com/boltdb/bolt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	DatabaseFilename = "consensus.db"
)

// initDatabase is run when the database. This has become the true
// init function for consensus set
func (cs *ConsensusSet) initDB(tx *bolt.Tx) error {
	// Set the block height to -1, so the genesis block is at height 0.
	blockHeight := tx.Bucket(BlockHeight)
	underflow := types.BlockHeight(0)
	err := blockHeight.Put(BlockHeight, encoding.Marshal(underflow-1))
	if err != nil {
		return err
	}

	// Add the genesis block.
	addBlockMap(tx, &cs.blockRoot)
	pushPath(tx, cs.blockRoot.Block.ID())

	// Set the siafund pool to 0.
	setSiafundPool(tx, types.NewCurrency64(0))

	// Update the siafundoutput diffs map for the genesis block on
	// disk. This needs to happen between the database being
	// opened/initilized and the consensus set hash being calculated
	for _, sfod := range cs.blockRoot.SiafundOutputDiffs {
		commitSiafundOutputDiff(tx, sfod, modules.DiffApply)
	}

	// Add the miner payout from the genesis block - unspendable, as the
	// unlock hash is blank.
	addSiacoinOutput(tx, cs.blockRoot.Block.MinerPayoutID(0), types.SiacoinOutput{
		Value:      types.CalculateCoinbase(0),
		UnlockHash: types.UnlockHash{},
	})

	// Get the genesis consensus set hash.
	if build.DEBUG {
		cs.blockRoot.ConsensusSetHash = consensusChecksum(tx)
	}

	// Add the genesis block to the block map.
	addBlockMap(tx, &cs.blockRoot)
	return nil
}

// loadDB pulls all the blocks that have been saved to disk into memory, using
// them to fill out the ConsensusSet.
func (cs *ConsensusSet) loadDB() error {
	db, err := openDB(filepath.Join(cs.persistDir, DatabaseFilename))
	if err != nil {
		return err
	}
	cs.db = db

	// Check the height. If the height is 0, then it's a new file and the
	// genesis block should be added.
	height := cs.db.pathHeight()
	if height == 0 {
		err := cs.db.Update(func(tx *bolt.Tx) error {
			return cs.initDB(tx)
		})
		if err != nil {
			return err
		}
	}

	// Check that the genesis block of the database matches the genesis block
	// generated during startup. If not, print a warning and create a new
	// database.
	bid := cs.db.getPath(0)
	if bid != cs.blockRoot.Block.ID() {
		println("WARNING: blockchain has wrong genesis block. A new blockchain will be created.")
		cs.db.Close()
		err = os.Rename(filepath.Join(cs.persistDir, DatabaseFilename), filepath.Join(cs.persistDir, DatabaseFilename+".bck"))
		if err != nil {
			return err
		}
		// Try to load again. Since the old database has been moved, the second
		// call will not follow this branch path again.
		return cs.loadDB()
	}

	// The state cannot be easily reverted to a point where the
	// consensusSetHash for the genesis block can be re-made. Load
	// from disk instead
	pb := cs.db.getBlockMap(bid)

	cs.blockRoot.ConsensusSetHash = pb.ConsensusSetHash

	return nil
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
