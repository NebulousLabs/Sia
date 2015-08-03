package consensus

import (
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// initDatabase is run when the database
func (cs *ConsensusSet) initSetDB() error {
	// add genesis block
	err := cs.db.addBlockMap(cs.blockRoot)
	if err != nil {
		return err
	}
	err = cs.db.pushPath(cs.blockRoot.Block.ID())
	if err != nil {
		return err
	}
	// Explicit initilization preferred to implicit
	cs.blocksLoaded = 0
	if build.DEBUG {
		cs.blockRoot.ConsensusSetHash = cs.consensusSetHash()
		cs.db.updateBlockMap(cs.blockRoot)
	}

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
	// consensusSetHash can be re-made. Load from disk instead
	pb := cs.db.getBlockMap(bid)

	cs.blockRoot.ConsensusSetHash = pb.ConsensusSetHash
	// Explicit initilization preferred to implicit
	cs.blocksLoaded = 0

	return nil
}

// loadDiffs is a transitional function to load the processed blocks
// from disk and move the diffs into memory
func (cs *ConsensusSet) loadDiffs() {
	height := cs.db.pathHeight()
	// load blocks from the db, starting after the genesis block
	for i := types.BlockHeight(1); i < height; i++ {
		bid := cs.db.getPath(i)
		pb := cs.db.getBlockMap(bid)

		// Blocks loaded from disk are trusted, don't bother with verification.
		lockID := cs.mu.Lock()
		// This guard is for when the program is stopped. It is temporary.
		// DEPRICATED
		if !cs.db.open {
			break
		}
		cs.updatePath = false
		cs.commitDiffSet(pb, modules.DiffApply)
		cs.updatePath = true
		cs.updateSubscribers(nil, []*processedBlock{pb})
		cs.mu.Unlock(lockID)
	}
}
