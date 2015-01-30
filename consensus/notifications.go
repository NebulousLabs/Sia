package consensus

import (
	"errors"
)

// OutputDiffsSince returns a set of output diffs representing how the state
// has changed since block `id`. OutputDiffsSince will flip the `new` value for
// diffs that got reversed.
func (s *State) BlocksSince(id BlockID) (removedBlocks, addedBlocks []BlockID, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	node, exists := s.blockMap[id]
	if !exists {
		err = errors.New("block is not known to the state or is an orphan")
		return
	}

	// Get all the ids from going backwards to the blockchain.
	reversedNodes := s.backtrackToBlockchain(node)
	height := reversedNodes[len(reversedNodes)-1].height
	// Eliminate the last node, which is the pivot node, whose diffs are already
	// known.
	reversedNodes = reversedNodes[:len(reversedNodes)-1]
	for _, reversedNode := range reversedNodes {
		removedBlocks = append(removedBlocks, reversedNode.block.ID())
	}

	// Get all the ids going forward from the pivot node.
	for _, exists := s.currentPath[height]; exists; height++ {
		node := s.blockMap[s.currentPath[height]]
		addedBlocks = append(addedBlocks, node.block.ID())
		_, exists = s.currentPath[height+1]
	}

	return
}
