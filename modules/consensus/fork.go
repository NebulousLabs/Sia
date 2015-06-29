package consensus

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// deleteNode recursively deletes its children from the set of known blocks.
// The node being deleted should not be a part of the current path.
func (cs *State) deleteNode(node *blockNode) {
	// Sanity check - the node being deleted should not be in the current path.
	if build.DEBUG {
		if types.BlockHeight(len(cs.currentPath)) > node.height &&
			cs.currentPath[node.height] == node.block.ID() {
			panic("cannot call 'deleteNode' on a node in the current path.")
		}
	}

	// Recusively call 'deleteNode' on of the input node's children, then
	// delete the input node.
	for i := range node.children {
		cs.deleteNode(node.children[i])
	}
	delete(cs.blockMap, node.block.ID())
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
// the current block. Blocks are returned in the order that they were reverted.
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

		// Sanity check - check that the delayed siacoin outputs map structure
		// matches the expected strucutre.
		if build.DEBUG {
			err := s.checkDelayedSiacoinOutputMaps()
			if err != nil {
				panic(err)
			}
		}
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

		// Sanity check - check that the delayed siacoin outputs map structure
		// matches the expected strucutre.
		if build.DEBUG {
			err = s.checkDelayedSiacoinOutputMaps()
			if err != nil {
				panic(err)
			}
		}
	}

	return
}

// forkBlockchain will move the consensus set onto the 'newNode' fork. An error
// will be returned if any of the blocks applied in the transition are found to
// be invalid. forkBlockchain is atomic; the State is only updated if the
// function returns nil.
func (cs *State) forkBlockchain(newNode *blockNode) (revertedNodes, appliedNodes []*blockNode, err error) {
	// In debug mode, record the old state hash before attempting the fork.
	// This variable is otherwise unused.
	var oldHash crypto.Hash
	if build.DEBUG {
		oldHash = cs.consensusSetHash()
	}
	oldHead := cs.currentBlockNode()

	// revert to the common parent
	commonParent := cs.backtrackToCurrentPath(newNode)[0]
	revertedNodes = cs.revertToNode(commonParent)

	// fast-forward to newNode
	appliedNodes, err = cs.applyUntilNode(newNode)
	if err == nil {
		return revertedNodes, appliedNodes, nil
	}

	// restore old path
	cs.revertToNode(commonParent)
	_, errReapply := cs.applyUntilNode(oldHead)
	if build.DEBUG {
		if errReapply != nil {
			panic("couldn't reapply previously applied diffs")
		} else if cs.consensusSetHash() != oldHash {
			panic("state hash changed after an unsuccessful fork attempt")
		}
	}
	return nil, nil, err
}
