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

// initDatabase is run when the database. This has become the true
// init function for consensus set
func (cs *ConsensusSet) initSetDB() error {
	// Set the block height to -1, adding the genesis block will bump it to 0.
	err := cs.db.Update(func(tx *bolt.Tx) error {
		blockHeight := tx.Bucket(BlockHeight)
		underflow := types.BlockHeight(0)
		return blockHeight.Put(BlockHeight, encoding.Marshal(underflow-1))
	})
	if err != nil {
		return err
	}

	// add genesis block
	err = cs.db.addBlockMap(&cs.blockRoot)
	if err != nil {
		return err
	}
	_ = cs.db.Update(func(tx *bolt.Tx) error {
		pushPath(tx, cs.blockRoot.Block.ID())
		return nil
	})

	// Set the siafund pool to 0.
	_ = cs.db.Update(func(tx *bolt.Tx) error {
		setSiafundPool(tx, types.NewCurrency64(0))
		return nil
	})

	// Update the siafundoutput diffs map for the genesis block on
	// disk. This needs to happen between the database being
	// opened/initilized and the consensus set hash being calculated
	_ = cs.db.Update(func(tx *bolt.Tx) error {
		for _, sfod := range cs.blockRoot.SiafundOutputDiffs {
			commitSiafundOutputDiff(tx, sfod, modules.DiffApply)
		}
		return nil
	})

	// Prevent the miner payout for the genesis block from being spent
	cs.db.addSiacoinOutputs(cs.blockRoot.Block.MinerPayoutID(0), types.SiacoinOutput{
		Value:      types.CalculateCoinbase(0),
		UnlockHash: types.UnlockHash{},
	})

	if build.DEBUG {
		cs.blockRoot.ConsensusSetHash = cs.consensusSetHash()
		cs.db.updateBlockMap(&cs.blockRoot)
	}
	return nil
}

// load pulls all the blocks that have been saved to disk into memory, using
// them to fill out the ConsensusSet.
func (cs *ConsensusSet) load() error {
	db, err := openDB(filepath.Join(cs.persistDir, "set.db"))
	if err != nil {
		return err
	}
	cs.db = db

	// Check the height. If the height is 0, then it's a new file and the
	// genesis block should be added.
	height := cs.db.pathHeight()
	if height == 0 {
		return cs.initSetDB()
	}

	// Check that the db's genesis block matches our genesis block.
	bid := cs.db.getPath(0)
	// If this happens, print a warning and start a new db.
	if bid != cs.blockRoot.Block.ID() {
		println("WARNING: blockchain has wrong genesis block. A new blockchain will be created.")
		cs.db.Close()
		err = os.Rename(filepath.Join(cs.persistDir, "set.db"), filepath.Join(cs.persistDir, "set.db.bck"))
		if err != nil {
			return err
		}
		// Try to load again. Since the old database has been moved, the second
		// call will not follow this branch path again.
		return cs.load()
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

	// Load all blocks from disk, skipping the genesis block.
	for i := types.BlockHeight(1); i < height; i++ {
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

	// Try to load an existing database from disk.
	err = cs.load()
	if err != nil {
		return err
	}

	// Send the genesis block to subscribers.
	cs.updateSubscribers(nil, []*processedBlock{&cs.blockRoot})

	// Send any blocks that were loaded from disk to subscribers.
	cs.loadDiffs()

	return nil
}
