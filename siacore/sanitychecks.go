package siacore

import (
	"fmt"
)

func CurrentPathCheck(s *State) {
	// Do a sanity check - check that every block is listed in CurrentPath and
	// that every block from current to genesis matches the block listed in
	// CurrentPath.
	currentNode := s.currentBlockNode()
	for i := s.Height(); ; i-- {
		// Check that the CurrentPath entry exists.
		id, exists := s.currentPath[i]
		if !exists {
			println(i)
			panic("current path is empty for a height with a known block.")
		}

		// Check that the CurrentPath entry contains the correct block id.
		if currentNode.Block.ID() != id {
			currentNodeID := currentNode.Block.ID()
			println(i)
			fmt.Println(id[:])
			fmt.Println(currentNodeID[:])
			panic("current path does not have correct id!")
		}

		currentNode = s.blockMap[currentNode.Block.ParentBlockID]

		// Have to do an awkward break beacuse i is unsigned.
		if i == 0 {
			break
		}
	}
}
