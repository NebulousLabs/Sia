package consensus

import (
	"errors"

	"github.com/boltdb/bolt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
)

var (
	errExternalRevert = errors.New("cannot revert to block outside of current path")
)

// backtrackToCurrentPath traces backwards from 'pb' until it reaches a node in
// the ConsensusSet's current path (the "common parent"). It returns the
// (inclusive) set of nodes between the common parent and 'pb', starting from
// the former.
func backtrackToCurrentPath(tx *bolt.Tx, pb *processedBlock) []*processedBlock {
	path := []*processedBlock{pb}
	for pb.Height > blockHeight(tx) || getPath(tx, pb.Height) != pb.Block.ID() {
		pb = getBlockMap(tx, pb.Parent)
		path = append([]*processedBlock{pb}, path...) // prepend
	}
	return path
}

// revertToBlock will revert blocks from the ConsensusSet's current path until
// 'pb' is the current block. Blocks are returned in the order that they were
// reverted.  'pb' is not reverted.
func revertToBlock(tx *bolt.Tx, pb *processedBlock) (revertedBlocks []*processedBlock) {
	// Sanity check - make sure that pb is in the current path.
	if build.DEBUG && (blockHeight(tx) < pb.Height || getPath(tx, pb.Height) != pb.Block.ID()) {
		panic(errExternalRevert)
	}

	// Rewind blocks until we reach 'pb'.
	for currentBlockID(tx) != pb.Block.ID() {
		node := currentProcessedBlock(tx)
		err := commitDiffSet(tx, node, modules.DiffRevert)
		if build.DEBUG && err != nil {
			panic(err)
		}
		revertedBlocks = append(revertedBlocks, node)
	}
	return revertedBlocks
}

// applyUntilBlock will successively apply the blocks between the consensus
// set's current path and 'pb'.
func (cs *ConsensusSet) applyUntilBlock(tx *bolt.Tx, pb *processedBlock) (appliedBlocks []*processedBlock, err error) {
	// Backtrack to the common parent of 'bn' and current path and then apply the new nodes.
	newPath := backtrackToCurrentPath(tx, pb)
	for _, node := range newPath[1:] {
		// If the diffs for this node have already been generated, apply diffs
		// directly instead of generating them. This is much faster.
		if node.DiffsGenerated {
			err := commitDiffSet(tx, node, modules.DiffApply)
			if err != nil {
				panic(err)
			}
		} else {
			err := generateAndApplyDiff(tx, node)
			if err != nil {
				// Mark the block as invalid.
				cs.dosBlocks[node.Block.ID()] = struct{}{}
				return nil, err
			}
		}
		appliedBlocks = append(appliedBlocks, node)
	}
	return appliedBlocks, nil
}

// forkBlockchain will move the consensus set onto the 'newBlock' fork. An error
// will be returned if any of the blocks applied in the transition are found to
// be invalid. forkBlockchain is atomic; the ConsensusSet is only updated if
// the function returns nil.
func (cs *ConsensusSet) forkBlockchain(tx *bolt.Tx, newBlock *processedBlock) (revertedBlocks, appliedBlocks []*processedBlock, err error) {
	commonParent := backtrackToCurrentPath(tx, newBlock)[0]
	revertedBlocks = revertToBlock(tx, commonParent)
	appliedBlocks, err = cs.applyUntilBlock(tx, newBlock)
	if err != nil {
		return nil, nil, err
	}
	return revertedBlocks, appliedBlocks, nil
}
