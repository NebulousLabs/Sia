package consensus

import (
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

// load pulls all the blocks that have been saved to disk into memory, using
// them to fill out the ConsensusSet.
func (cs *ConsensusSet) load(saveDir string) error {
	db, err := persist.OpenDB(filepath.Join(saveDir, "chain.db"))
	if err != nil {
		return err
	}

	// Check the height. If the height is 0, then it's a new file and the
	// genesis block should be added.
	height, err := db.Height()
	if err != nil {
		return err
	}
	if height == 0 {
		// add genesis block
		cs.db = db
		return db.AddBlock(cs.blockRoot.block)
	}

	// Check that the db's genesis block matches our genesis block.
	b, err := db.Block(0)
	if err != nil {
		return err
	}
	// If this happens, print a warning and start a new db.
	if b.ID() != cs.currentPath[0] {
		println("WARNING: blockchain has wrong genesis block. A new blockchain will be created.")
		db.Close()
		err := os.Rename(filepath.Join(saveDir, "chain.db"), filepath.Join(saveDir, "chain.db.bck"))
		if err != nil {
			return err
		}
		// Now that chain.db no longer exists, recursing will create a new
		// empty db and add the genesis block to it.
		return cs.load(saveDir)
	}

	// load blocks from the db, starting after the genesis block
	// NOTE: during load, the state uses the NilDB. This prevents AcceptBlock
	// from adding duplicate blocks to the real database.
	cs.db = persist.NilDB
	for i := types.BlockHeight(1); i < height; i++ {
		b, err := db.Block(i)
		if err != nil {
			// should never happen
			return err
		}

		// Blocks loaded from disk are trusted, don't bother with verification.
		lockID := cs.mu.Lock()
		cs.verificationRigor = partialVerification
		err = cs.acceptBlock(b)
		cs.mu.Unlock(lockID)
		if err != nil {
			return err
		}
	}
	// start using the real db
	cs.db = db
	return nil
}
