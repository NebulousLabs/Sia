package consensus

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	errDeleteCurrentPath = errors.New("cannot call 'deleteNode' on a block node in the current path")
	errExternalRevert    = errors.New("cannot revert to node outside of current path")
)

// deleteNode recursively deletes its children from the set of known blocks.
// The node being deleted should not be a part of the current path.
func (cs *ConsensusSet) deleteNode(bn *blockNode) {
	// Sanity check - the node being deleted should not be in the current path.
	if build.DEBUG {
		if types.BlockHeight(cs.db.pathHeight()) > bn.height &&
			cs.db.getPath(bn.height) == bn.block.ID() {
			panic(errDeleteCurrentPath)
		}
	}

	// Recusively call 'deleteNode' on of the input node's children.
	for i := range bn.children {
		cs.deleteNode(bn.children[i])
	}

	// Remove the node from the block map, and from its parents list of
	// children.
	delete(cs.blockMap, bn.block.ID())
	cs.db.rmBlockMap(bn.block.ID())
	for i := range bn.parent.children {
		if bn.parent.children[i] == bn {
			// If 'i' is not the last element, remove it from the array by
			// copying the remaining array over it.
			if i < len(bn.parent.children)-1 {
				copy(bn.parent.children[i:], bn.parent.children[i+1:])
			}
			// Trim the last element.
			bn.parent.children = bn.parent.children[:len(bn.parent.children)-1]
			break
		}
	}
}

// backtrackToCurrentPath traces backwards from 'bn' until it reaches a node in
// the ConsensusSet's current path (the "common parent"). It returns the
// (inclusive) set of nodes between the common parent and 'bn', starting from
// the former.
func (cs *ConsensusSet) backtrackToCurrentPath(bn *blockNode) []*blockNode {
	path := []*blockNode{bn}
	for {
		// Stop when we reach the common parent.
		if bn.height <= cs.height() && cs.db.getPath(bn.height) == bn.block.ID() {
			break
		}
		bn = bn.parent
		path = append([]*blockNode{bn}, path...) // prepend
	}
	return path
}

// revertToNode will revert blocks from the ConsensusSet's current path until
// 'bn' is the current block. Blocks are returned in the order that they were
// reverted.  'bn' is not reverted.
func (cs *ConsensusSet) revertToNode(bn *blockNode) (revertedNodes []*blockNode) {
	// Sanity check - make sure that bn is in the current path.
	if build.DEBUG {
		if cs.height() < bn.height || cs.db.getPath(bn.height) != bn.block.ID() {
			panic(errExternalRevert)
		}
	}

	// Rewind blocks until we reach 'bn'.
	for cs.currentBlockID() != bn.block.ID() {
		node := cs.currentBlockNode()
		bn := bnToPb(*node)
		cs.commitDiffSet(&bn, modules.DiffRevert)
		revertedNodes = append(revertedNodes, node)
	}
	return
}

// applyUntilNode will successively apply the blocks between the consensus
// set's current path and 'bn'.
func (s *ConsensusSet) applyUntilNode(bn *blockNode) (appliedNodes []*blockNode, err error) {
	// Backtrack to the common parent of 'bn' and current path and then apply the new nodes.
	newPath := s.backtrackToCurrentPath(bn)
	for _, node := range newPath[1:] {
		// If the diffs for this node have already been generated, apply diffs
		// directly instead of generating them. This is much faster.
		if node.diffsGenerated {
			bn := bnToPb(*node)
			s.commitDiffSet(&bn, modules.DiffApply)
		} else {
			bn := bnToPb(*node)
			err = s.generateAndApplyDiff(&bn)
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
// be invalid. forkBlockchain is atomic; the ConsensusSet is only updated if
// the function returns nil.
func (cs *ConsensusSet) forkBlockchain(newNode *blockNode) (revertedNodes, appliedNodes []*blockNode, err error) {
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
