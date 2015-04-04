package consensus

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

// deleteNode recursively deletes its children from the set of known blocks.
func (s *State) deleteNode(node *blockNode) {
	for i := range node.children {
		s.deleteNode(node.children[i])
	}
	delete(s.blockMap, node.block.ID())
}

// backtrackToCurrentPath traces backwards from 'bn' until it reaches a node in
// the State's current path (the "common parent"). It returns the (inclusive)
// set of nodes between the common parent and 'bn', starting from the former.
func (s *State) backtrackToCurrentPath(bn *blockNode) []*blockNode {
	path := []*blockNode{bn}
	for {
		// stop when we reach the common parent
		if bn.height <= s.height() && s.currentPath[bn.height] == bn.block.ID() {
			break
		}

		bn = bn.parent
		path = append([]*blockNode{bn}, path...) // prepend, not append

		// Sanity check - all block nodes should have a parent except the
		// genesis block, and this loop should break before reaching the
		// genesis block.
		if bn == nil {
			if build.DEBUG {
				panic("backtrack hit a nil node?")
			}
			break
		}
	}
	return path
}

// revertToNode will revert blocks from the State's current path until 'bn' is
// the current block.
func (s *State) revertToNode(bn *blockNode) (revertedNodes []*blockNode) {
	// Sanity check - make sure that bn is in the currentPath.
	if build.DEBUG {
		if s.currentPath[bn.height] != bn.block.ID() {
			panic("can't revert to node not in current path")
		}
	}

	// Rewind blocks until we reach 'bn'.
	for s.currentBlockID() != bn.block.ID() {
		node := s.currentBlockNode()
		s.commitDiffSet(node, modules.DiffRevert)
		revertedNodes = append(revertedNodes, node)
	}
	return
}

// applyUntilNode will successively apply the blocks between the state's
// currentPath and 'bn'.
func (s *State) applyUntilNode(bn *blockNode) (appliedNodes []*blockNode, err error) {
	// Backtrack to the common parent of 'bn' and currentPath.
	newPath := s.backtrackToCurrentPath(bn)

	// Apply new nodes.
	for _, node := range newPath[1:] {
		// If the diffs for this node have already been generated, apply diffs
		// directly instead of generating them. This is much faster.
		if node.diffsGenerated {
			s.commitDiffSet(node, modules.DiffApply)
		} else {
			err = s.generateAndApplyDiff(node)
			if err != nil {
				break
			}
		}
		appliedNodes = append(appliedNodes, node)
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
	if build.DEBUG {
		oldHash = s.consensusSetHash()
	}
	oldHead := s.currentBlockNode()

	// revert to the common parent
	commonParent := s.backtrackToCurrentPath(newNode)[0]
	revertedNodes := s.revertToNode(commonParent)

	// fast-forward to newNode
	appliedNodes, err := s.applyUntilNode(newNode)
	if err == nil {
		// If application succeeded, notify the subscribers and return. Error
		// handling happens outside this if statement.
		s.updateSubscribers(revertedNodes, appliedNodes)
		return
	}

	// restore old path
	s.revertToNode(commonParent)
	_, errReapply := s.applyUntilNode(oldHead)
	if build.DEBUG {
		if errReapply != nil {
			panic("couldn't reapply previously applied diffs")
		} else if s.consensusSetHash() != oldHash {
			panic("state hash changed after an unsuccessful fork attempt")
		}
	}
	return
}
