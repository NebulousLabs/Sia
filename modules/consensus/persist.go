package consensus

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/blockdb"
	"github.com/NebulousLabs/Sia/types"
)

func (s *State) load(saveDir string) error {
	var err error
	s.db, err = blockdb.Open(filepath.Join(saveDir, "chain.db"))
	if err != nil {
		return err
	}
	height, err := s.db.Height()
	if err != nil {
		return err
	}
	if height == 0 {
		// add genesis block
		return s.db.AddBlock(s.blockMap[s.currentPath[0]].block)
	}
	for i := types.BlockHeight(0); i < height; i++ {
		b, err := s.db.Block(i)
		if err != nil {
			// should never happen
			return err
		}
		err = s.AcceptBlock(b)
		if err != nil {
			return err
		}
	}
	return nil
}
