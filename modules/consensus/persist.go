package consensus

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/boltdb/bolt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// initDatabase is run when the database. This has become the true
// init function for consensus set
func (cs *ConsensusSet) initSetDB() error {
	err := cs.db.startConsistencyGuard()
	if err != nil {
		return err
	}

	// add genesis block
	err = cs.db.addBlockMap(&cs.blockRoot)
	if err != nil {
		return err
	}
	err = cs.db.pushPath(cs.blockRoot.Block.ID())
	if err != nil {
		return err
	}

	// Set the siafund pool to 0.
	err = cs.db.Update(func(tx *bolt.Tx) error {
		sfpBucket := tx.Bucket(SiafundPool)
		return sfpBucket.Put(SiafundPool, encoding.Marshal(types.NewCurrency64(0)))
	})
	if err != nil {
		return err
	}

	// Update the siafundoutput diffs map for the genesis block on
	// disk. This needs to happen between the database being
	// opened/initilized and the consensus set hash being calculated
	err = cs.db.Update(func(tx *bolt.Tx) error {
		for _, sfod := range cs.blockRoot.SiafundOutputDiffs {
			err := cs.commitTxSiafundOutputDiff(tx, sfod, modules.DiffApply)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Prevent the miner payout for the genesis block from being spent
	cs.db.addSiacoinOutputs(cs.blockRoot.Block.MinerPayoutID(0), types.SiacoinOutput{
		Value:      types.CalculateCoinbase(0),
		UnlockHash: types.UnlockHash{},
	})

	if build.DEBUG {
		cs.blockRoot.ConsensusSetHash = cs.consensusSetHash()
		cs.db.updateBlockMap(&cs.blockRoot)
	}
	cs.db.stopConsistencyGuard()
	return nil
}

// load pulls all the blocks that have been saved to disk into memory, using
// them to fill out the ConsensusSet.
func (cs *ConsensusSet) load(saveDir string) error {
	db, err := openDB(filepath.Join(saveDir, "set.db"))
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
		err = os.Rename(filepath.Join(saveDir, "set.db"), filepath.Join(saveDir, "set.db.bck"))
		if err != nil {
			return err
		}
		// Now that chain.db no longer exists, recursing will create a new
		// empty db and add the genesis block to it.
		return cs.load(saveDir)
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

		// Yield the processor so that other goroutines have a chance to grab
		// the lock before it is immeditately grabbed again in the tight loop.
		runtime.Gosched()
	}

	// Do a consistency check after loading the database.
	if height > 1 && build.DEBUG {
		err := cs.db.startConsistencyGuard()
		if err != nil {
			panic(err)
		}
		err = cs.checkConsistency()
		if err != nil {
			panic(err)
		}
		cs.db.stopConsistencyGuard()
	}
}
