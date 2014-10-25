package sia

import (
	"errors"
)

// Currently a stateless verification. State is needed to build a tree though.
func BlockVerify(b Block) {

}

// Add a block to the state struct.
func (s *State) IncorporateBlock(b Block) (err error) {
	bid := b.ID()

	_, exists := s.BadBlocks[bid]
	if exists {
		err = errors.New("Block is in bad list")
		return
	}

	if b.Version != 1 {
		s.BadBlocks[bid] = struct{}{}
		err = errors.New("Block is not version 1")
		return
	}

	// If timestamp is in the future, return an error and store in future blocks list?
	// Certainly it doesn't belong in bad blocks, but...

	_, exists = s.BlockMap[bid]
	if exists {
		err = errors.New("Block exists in block map.")
		return
	}

	_, exists = s.OrphanBlocks[bid]
	if exists {
		err = errors.New("Block exists in orphan list")
		return
	}

	prevNode, exists := s.BlockMap[b.Prevblock]
	if !exists {
		OrphanBlocks[bid] = b
		err = errors.New("Block is a new orphan")
		return
	}

	// Check that the root is good.
}
