package consensus

import (
	"errors"
	"fmt"

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
func (cs *ConsensusSet) deleteNode(pb *processedBlock) {
	// Sanity check - the node being deleted should not be in the current path.
	if build.DEBUG {
		if types.BlockHeight(cs.db.pathHeight()) > pb.Height &&
			cs.db.getPath(pb.Height) == pb.Block.ID() {
			panic(errDeleteCurrentPath)
		}
	}

	// Recusively call 'deleteNode' on of the input node's children.
	for i := range pb.Children {
		child := cs.db.getBlockMap(pb.Children[i])
		cs.deleteNode(child)
	}

	// Remove the node from the block map, and from its parents list of
	// children.
	cs.db.rmBlockMap(pb.Block.ID())
	parent := cs.db.getBlockMap(pb.Parent)
	for i := range parent.Children {
		if parent.Children[i] == pb.Block.ID() {
			// If 'i' is not the last element, remove it from the array by
			// copying the remaining array over it.
			if i < len(parent.Children)-1 {
				copy(parent.Children[i:], parent.Children[i+1:])
			}
			// Trim the last element.
			parent.Children = parent.Children[:len(parent.Children)-1]
			break
		}
	}
	cs.db.updateBlockMap(parent)
}

// backtrackToCurrentPath traces backwards from 'pb' until it reaches a node in
// the ConsensusSet's current path (the "common parent"). It returns the
// (inclusive) set of nodes between the common parent and 'pb', starting from
// the former.
func (cs *ConsensusSet) backtrackToCurrentPath(pb *processedBlock) []*processedBlock {
	path := []*processedBlock{pb}
	for {
		// Stop when we reach the common parent.
		if pb.Height <= cs.height() && cs.db.getPath(pb.Height) == pb.Block.ID() {
			break
		}
		pb = cs.db.getBlockMap(pb.Parent)
		path = append([]*processedBlock{pb}, path...) // prepend
	}
	fmt.Printf("Backtracking path:\n")
	for _, b := range path {
		fmt.Printf("    %x\n", b.Block.ID())
	}
	return path
}

// revertToNode will revert blocks from the ConsensusSet's current path until
// 'pb' is the current block. Blocks are returned in the order that they were
// reverted.  'pb' is not reverted.
func (cs *ConsensusSet) revertToNode(pb *processedBlock) (revertedNodes []*processedBlock) {
	// Sanity check - make sure that pb is in the current path.
	if build.DEBUG {
		if cs.height() < pb.Height || cs.db.getPath(pb.Height) != pb.Block.ID() {
			panic(errExternalRevert)
		}
	}

	// Rewind blocks until we reach 'pb'.
	for cs.currentBlockID() != pb.Block.ID() {
		node := cs.currentProcessedBlock()
		cs.commitDiffSet(node, modules.DiffRevert)
		fmt.Printf("Reverted %x\n", node.Block.ID())
		revertedNodes = append(revertedNodes, node)
	}
	return
}

// applyUntilNode will successively apply the blocks between the consensus
// set's current path and 'pb'.
func (s *ConsensusSet) applyUntilNode(pb *processedBlock) (appliedBlocks []*processedBlock, err error) {
	// Backtrack to the common parent of 'bn' and current path and then apply the new nodes.
	newPath := s.backtrackToCurrentPath(pb)
	fmt.Printf("new path:\n")
	for _, b := range newPath {
		fmt.Printf("    %x\n", b.Block.ID())
	}
	for _, node := range newPath[1:] {
		// If the diffs for this node have already been generated, apply diffs
		// directly instead of generating them. This is much faster.
		fmt.Printf("Applying %x, with diffs %d\n", node.Block.ID(), node.DiffsGenerated)
		if node.DiffsGenerated {
			s.commitDiffSet(pb, modules.DiffApply)
		} else {
			err = s.generateAndApplyDiff(pb)
			if err != nil {
				break
			}
		}
		appliedBlocks = append(appliedBlocks, node)
	}
	return appliedBlocks, err
}

// forkBlockchain will move the consensus set onto the 'newNode' fork. An error
// will be returned if any of the blocks applied in the transition are found to
// be invalid. forkBlockchain is atomic; the ConsensusSet is only updated if
// the function returns nil.
func (cs *ConsensusSet) forkBlockchain(newNode *processedBlock) (revertedNodes, appliedNodes []*processedBlock, err error) {
	// In debug mode, record the old state hash before attempting the fork.
	// This variable is otherwise unused.
	var oldHash crypto.Hash
	if build.DEBUG {
		oldHash = cs.consensusSetHash()
	}
	oldHead := cs.currentProcessedBlock()

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
