package consensus

import (
	"github.com/NebulousLabs/Sia/crypto"
)

// invalidateNode recursively moves the children of a blockNode to the bad
// blocks list.
func (s *State) invalidateNode(node *blockNode) {
	for i := range node.children {
		s.invalidateNode(node.children[i])
	}

	delete(s.blockMap, node.block.ID())
	s.badBlocks[node.block.ID()] = struct{}{}
}

// backtrackToBlockchain traces backwards from 'bn' until it reaches a node in
// the State's current path (the "common parent"). It returns the (inclusive)
// set of nodes between the common parent and 'bn', starting from the former.
func (s *State) backtrackToBlockchain(bn *blockNode) []*blockNode {
	path := []*blockNode{bn}
	for s.currentPath[bn.height] != bn.block.ID() {
		bn = bn.parent
		// prepend, not append
		path = append([]*blockNode{bn}, path...)

		// Sanity check - all block nodes should have a parent except the
		// genesis block, and this loop should break before reaching the
		// genesis block.
		if bn == nil {
			if DEBUG {
				panic("backtrack hit a nil node?")
			}
			break
		}
	}
	return path
}

// rewindToNode will rewind blocks from the State's current path until 'bn' is
// the current block.
func (s *State) rewindToNode(bn *blockNode) {
	// Sanity check - make sure that bn is in the currentPath.
	if DEBUG {
		if s.currentPath[bn.height] != bn.block.ID() {
			panic("can't rewind to node not in current path")
		}
	}

	// Rewind blocks until we reach 'bn'.
	for s.currentBlockID != bn.block.ID() {
		cur := s.currentBlockNode()
		s.commitDiffSet(cur, DiffRevert)
	}
}

// applyUntilNode will successively apply the blocks between the state's
// currentPath and 'bn'.
func (s *State) applyUntilNode(bn *blockNode) (err error) {
	// Backtrack to the common parent of 'bn' and currentPath.
	newPath := s.backtrackToBlockchain(bn)

	// Apply new nodes.
	for _, node := range newPath[1:] {
		// If the diffs for this node have already been generated, apply diffs
		// directly instead of generating them. This is much faster.
		if node.diffsGenerated {
			s.commitDiffSet(node, DiffApply)
		} else {
			err = s.generateAndApplyDiff(node)
			if err != nil {
				break
			}
		}
	}

	return
}

// forkBlockchain will move the consensus set onto the 'newNode' fork. An error
// will be returned if any of the blocks applied in the transition are found to
// be invalid. forkBlockchain is atomic; the State is only updated if the
// function returns nil.
func (s *State) forkBlockchain(newNode *blockNode) (err error) {
	// In debug mode, record the old state hash before attempting the fork.
	// This variable is otherwise unused.
	var oldHash crypto.Hash
	if DEBUG {
		oldHash = s.stateHash()
	}

	// determine common parent
	commonParent := s.backtrackToBlockchain(newNode)[0]

	// save currentBlockNode in case something goes wrong
	oldHead := s.currentBlockNode()

	// rewind to the common parent
	s.rewindToNode(commonParent)

	// fast-forward to newNode
	err = s.applyUntilNode(newNode)
	if err != nil {
		// restore old path
		s.rewindToNode(commonParent)
		errReapply := s.applyUntilNode(oldHead)
		if DEBUG {
			if errReapply != nil {
				panic("couldn't reapply previously applied diffs!")
			} else if s.stateHash() != oldHash {
				panic("state hash changed after an unsuccessful fork attempt")
			}
		}
		return
	}

	return
}
