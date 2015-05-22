package consensus

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

// load pulls all the blocks that have been saved to disk into memory, using
// them to fill out the State.
func (s *State) load(saveDir string) error {
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
		s.db = db
		return db.AddBlock(s.blockMap[s.currentPath[0]].block)
	}

	// load blocks from the db, starting after the genesis block
	// NOTE: during load, the state uses the NilDB. This prevents AcceptBlock
	// from adding duplicate blocks to the real database.
	s.db = persist.NilDB
	for i := types.BlockHeight(1); i < height; i++ {
		b, err := db.Block(i)
		if err != nil {
			// should never happen
			return err
		}

		// Blocks loaded from disk are trusted, don't bother with verification.
		lockID := s.mu.Lock()
		s.fullVerification = false
		err = s.acceptBlock(b)
		s.mu.Unlock(lockID)
		if err != nil {
			return err
		}
	}
	// start using the real db
	s.db = db
	return nil
}
