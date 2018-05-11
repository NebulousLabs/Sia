package consensus

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus/database"
)

var (
	errExternalRevert = errors.New("cannot revert to block outside of current path")
)

// backtrackToCurrentPath traces backwards from 'b' until it reaches a block
// in the ConsensusSet's current path (the "common parent"). It returns the
// (inclusive) set of blocks between the common parent and 'b', starting from
// the former.
func backtrackToCurrentPath(tx database.Tx, b *database.Block) []*database.Block {
	path := []*database.Block{b}
	for {
		// Error is not checked in production code - an error can only indicate
		// that b.Height > blockHeight(tx).
		currentPathID, err := getPath(tx, b.Height)
		if currentPathID == b.ID() {
			break
		}
		// Sanity check - an error should only indicate that b.Height >
		// blockHeight(tx).
		if build.DEBUG && err != nil && b.Height <= tx.BlockHeight() {
			panic(err)
		}

		// Prepend the next block to the list of blocks leading from the
		// current path to the input block.
		b, err = getBlockMap(tx, b.ParentID)
		if build.DEBUG && err != nil {
			panic(err)
		}
		path = append([]*database.Block{b}, path...)
	}
	return path
}

// revertToBlock will revert blocks from the ConsensusSet's current path until
// 'b' is the current block. Blocks are returned in the order that they were
// reverted.  'b' is not reverted.
func (cs *ConsensusSet) revertToBlock(tx database.Tx, b *database.Block) (revertedBlocks []*database.Block) {
	// Sanity check - make sure that b is in the current path.
	currentPathID, err := getPath(tx, b.Height)
	if build.DEBUG && (err != nil || currentPathID != b.ID()) {
		panic(errExternalRevert)
	}

	// Rewind blocks until 'b' is the current block.
	for currentBlockID(tx) != b.ID() {
		block := currentProcessedBlock(tx)
		commitDiffSet(tx, block, modules.DiffRevert)
		revertedBlocks = append(revertedBlocks, block)

		// Sanity check - after removing a block, check that the consensus set
		// has maintained consistency.
		if build.Release == "testing" {
			cs.checkConsistency(tx)
		} else {
			cs.maybeCheckConsistency(tx)
		}
	}
	return revertedBlocks
}

// applyUntilBlock will successively apply the blocks between the consensus
// set's current path and 'b'.
func (cs *ConsensusSet) applyUntilBlock(tx database.Tx, b *database.Block) (appliedBlocks []*database.Block, err error) {
	// Backtrack to the common parent of 'bn' and current path and then apply the new blocks.
	newPath := backtrackToCurrentPath(tx, b)
	for _, block := range newPath[1:] {
		// If the diffs for this block have already been generated, apply diffs
		// directly instead of generating them. This is much faster.
		if block.DiffsGenerated {
			commitDiffSet(tx, block, modules.DiffApply)
		} else {
			err := generateAndApplyDiff(tx, block)
			if err != nil {
				// Mark the block as invalid.
				cs.dosBlocks[block.Block.ID()] = struct{}{}
				return nil, err
			}
		}
		appliedBlocks = append(appliedBlocks, block)

		// Sanity check - after applying a block, check that the consensus set
		// has maintained consistency.
		if build.Release == "testing" {
			cs.checkConsistency(tx)
		} else {
			cs.maybeCheckConsistency(tx)
		}
	}
	return appliedBlocks, nil
}

// forkBlockchain will move the consensus set onto the 'newBlock' fork. An
// error will be returned if any of the blocks applied in the transition are
// found to be invalid. forkBlockchain is atomic; the ConsensusSet is only
// updated if the function returns nil.
func (cs *ConsensusSet) forkBlockchain(tx database.Tx, newBlock *database.Block) (revertedBlocks, appliedBlocks []*database.Block, err error) {
	commonParent := backtrackToCurrentPath(tx, newBlock)[0]
	revertedBlocks = cs.revertToBlock(tx, commonParent)
	appliedBlocks, err = cs.applyUntilBlock(tx, newBlock)
	if err != nil {
		return nil, nil, err
	}
	return revertedBlocks, appliedBlocks, nil
}
