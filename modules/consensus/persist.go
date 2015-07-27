package consensus

import (
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// load pulls all the blocks that have been saved to disk into memory, using
// them to fill out the ConsensusSet.
func (cs *ConsensusSet) load(saveDir string) error {
	db, err := openDB(filepath.Join(saveDir, "set.db"))
	cs.db = db

	if err != nil {
		return err
	}

	// Check the height. If the height is 0, then it's a new file and the
	// genesis block should be added.
	height := db.pathHeight()
	if height == 0 {
		// add genesis block
		return db.pushPath(cs.blockRoot.block)
	}

	// Check that the db's genesis block matches our genesis block.
	bID := db.getPath(0)
	// If this happens, print a warning and start a new db.
	if bid != cs.currentPath[0] {
		println("WARNING: blockchain has wrong genesis block. A new blockchain will be created.")
		db.Close()
		err = os.Rename(filepath.Join(saveDir, "set.db"), filepath.Join(saveDir, "set.db.bck"))
		if err != nil {
			return err
		}
		// Now that chain.db no longer exists, recursing will create a new
		// empty db and add the genesis block to it.
		return cs.load(saveDir)
	}

	// load blocks from the db, starting after the genesis block
	for i := types.BlockHeight(1); i < height; i++ {
		bID := db.getPath(i)
		pb, err := db.getBlockMap(bID)
		if err != nil {
			return err
		}
		bn := cs.pbToBn(pb)

		// Blocks loaded from disk are trusted, don't bother with verification.
		lockID := cs.mu.Lock()
		cs.blockMap[bn.block.ID()] = &bn
		cs.updatePath = false
		cs.commitDiffSet(&bn, modules.DiffApply)
		cs.updatePath = true
		cs.updateSubscribers(nil, []*blockNode{&bn})
		cs.mu.Unlock(lockID)
	}
	return nil
}
