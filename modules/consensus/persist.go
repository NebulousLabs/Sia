package consensus

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/blockdb"
	"github.com/NebulousLabs/Sia/types"
)

func (s *State) load(saveDir string) error {
	db, err := blockdb.Open(filepath.Join(saveDir, "chain.db"))
	if err != nil {
		return err
	}
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
	s.db = blockdb.NilDB
	for i := types.BlockHeight(1); i < height; i++ {
		b, err := db.Block(i)
		if err != nil {
			// should never happen
			return err
		}
		err = s.AcceptBlock(b)
		if err == ErrBlockKnown {
			println(i)
		} else if err != nil {
			return err
		}
	}
	// start using the real db
	s.db = db
	return nil
}
