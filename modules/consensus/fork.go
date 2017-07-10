package consensus

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/bolt"
)

var (
	errExternalRevert = errors.New("cannot revert to block outside of current path")
)

// backtrackToCurrentPath traces backwards from 'pb' until it reaches a block
// in the ConsensusSet's current path (the "common parent"). It returns the
// (inclusive) set of blocks between the common parent and 'pb', starting from
// the former.
func backtrackToCurrentPath(tx *bolt.Tx, pb *processedBlock) []*processedBlock {
	path := []*processedBlock{pb}
	for {
		// Error is not checked in production code - an error can only indicate
		// that pb.Height > blockHeight(tx).
		currentPathID, err := getPath(tx, pb.Height)
		if currentPathID == pb.Block.ID() {
			break
		}
		// Sanity check - an error should only indicate that pb.Height >
		// blockHeight(tx).
		if build.DEBUG && err != nil && pb.Height <= blockHeight(tx) {
			panic(err)
		}

		// Prepend the next block to the list of blocks leading from the
		// current path to the input block.
		pb, err = getBlockMap(tx, pb.Block.ParentID)
		if build.DEBUG && err != nil {
			panic(err)
		}
		path = append([]*processedBlock{pb}, path...)
	}
	return path
}

// backtrackToCurrentPath traces backwards from 'pb' until it reaches a block
// in the ConsensusSet's current path (the "common parent"). It returns the
// (inclusive) set of blocks between the common parent and 'pb', starting from
// the former.
func backtrackHeadersToCurrentPath(tx *bolt.Tx, ph *processedHeader) []*processedHeader {
	path := []*processedHeader{ph}
	for {
		// Error is not checked in production code - an error can only indicate
		// that pb.Height > blockHeight(tx).
		currentPathID, err := getPath(tx, ph.Height)
		if currentPathID == ph.BlockHeader.ID() {
			break
		}
		// Sanity check - an error should only indicate that pb.Height >
		// blockHeight(tx).
		if build.DEBUG && err != nil && ph.Height <= blockHeight(tx) {
			panic(err)
		}

		// Prepend the next block to the list of blocks leading from the
		// current path to the input block.
		ph, err = getHeaderMap(tx, ph.BlockHeader.ParentID)
		if build.DEBUG && err != nil {
			panic(err)
		}
		path = append([]*processedHeader{ph}, path...)
	}
	return path
}

// revertToBlock will revert blocks from the ConsensusSet's current path until
// 'pb' is the current block. Blocks are returned in the order that they were
// reverted.  'pb' is not reverted.
func (cs *ConsensusSet) revertToBlock(tx *bolt.Tx, pb *processedBlock) (revertedBlocks []*processedBlock) {
	// Sanity check - make sure that pb is in the current path.
	currentPathID, err := getPath(tx, pb.Height)
	if build.DEBUG && (err != nil || currentPathID != pb.Block.ID()) {
		panic(errExternalRevert)
	}

	// Rewind blocks until 'pb' is the current block.
	for currentBlockID(tx) != pb.Block.ID() {
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

// revertToBlock will revert blocks from the ConsensusSet's current path until
// 'ph' is the current block. Blocks are returned in the order that they were
// reverted.  'ph' is not reverted.
func (cs *ConsensusSet) revertToHeader(tx *bolt.Tx, ph *processedHeader) (revertedHeaders []*processedHeader) {
	// Sanity check - make sure that pb is in the current path.
	currentPathID, err := getPath(tx, ph.Height)
	if build.DEBUG && (err != nil || currentPathID != ph.BlockHeader.ID()) {
		panic(errExternalRevert)
	}

	// Rewind blocks until 'ph' is the current block.
	curr := currentBlockID(tx)
	for curr != ph.BlockHeader.ID() {
		header := currentProcessedHeader(tx)
		revertedHeaders = append(revertedHeaders, header)

		// Sanity check - after removing a block, check that the consensus set
		// has maintained consistency.
		if build.Release == "testing" {
			cs.checkConsistency(tx)
		} else {
			cs.maybeCheckConsistency(tx)
		}
	}
	return revertedHeaders
}

// applyUntilBlock will successively apply the blocks between the consensus
// set's current path and 'pb'.
func (cs *ConsensusSet) applyUntilBlock(tx *bolt.Tx, pb *processedBlock) (appliedBlocks []*processedBlock, err error) {
	// Backtrack to the common parent of 'bn' and current path and then apply the new blocks.
	newPath := backtrackToCurrentPath(tx, pb)
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

// applyUntilBlock will successively apply the blocks between the consensus
// set's current path and 'ph'.
func (cs *ConsensusSet) applyUntilHeader(tx *bolt.Tx, ph *processedHeader) (headers []*processedHeader) {
	// Backtrack to the common parent of 'bn' and current path and then apply the new blocks.
	newPath := backtrackHeadersToCurrentPath(tx, ph)
	for _, header := range newPath[1:] {
		headerMap := tx.Bucket(HeaderMap)
		id := ph.BlockHeader.ID()

		headerMap.Put(id[:], encoding.Marshal(*header))
		headers = append(headers, header)

		// Sanity check - after applying a block, check that the consensus set
		// has maintained consistency.
		if build.Release == "testing" {
			cs.checkConsistency(tx)
		} else {
			cs.maybeCheckConsistency(tx)
		}
	}
	return headers
}

// forkBlockchain will move the consensus set onto the 'newBlock' fork. An
// error will be returned if any of the blocks applied in the transition are
// found to be invalid. forkBlockchain is atomic; the ConsensusSet is only
// updated if the function returns nil.
func (cs *ConsensusSet) forkBlockchain(tx *bolt.Tx, newBlock *processedBlock) (revertedBlocks, appliedBlocks []*processedBlock, err error) {
	commonParent := backtrackToCurrentPath(tx, newBlock)[0]
	revertedBlocks = cs.revertToBlock(tx, commonParent)
	appliedBlocks, err = cs.applyUntilBlock(tx, newBlock)
	if err != nil {
		return nil, nil, err
	}
	return revertedBlocks, appliedBlocks, nil
}

// forkBlockchain will move the consensus set onto the 'newHeaders' fork. An
// error will be returned if any of the blocks headers in the transition are
// found to be invalid. forkHeadersBlockchain is atomic; the ConsensusSet is only
// updated if the function returns nil.
func (cs *ConsensusSet) forkHeadersBlockchain(tx *bolt.Tx, newHeader *processedHeader) (revertedBlocks, appliedHeaders []*processedHeader) {
	commonParent := backtrackHeadersToCurrentPath(tx, newHeader)[0]
	revertedBlocks = cs.revertToHeader(tx, commonParent)
	appliedHeaders = cs.applyUntilHeader(tx, newHeader)
	return revertedBlocks, appliedHeaders
}
