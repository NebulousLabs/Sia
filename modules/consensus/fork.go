package consensus

import (
	"errors"

	"github.com/boltdb/bolt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

var (
	errExternalRevert = errors.New("cannot revert to node outside of current path")
)

// backtrackToCurrentPath traces backwards from 'pb' until it reaches a node in
// the ConsensusSet's current path (the "common parent"). It returns the
// (inclusive) set of nodes between the common parent and 'pb', starting from
// the former.
func backtrackToCurrentPath(tx *bolt.Tx, pb *processedBlock) []*processedBlock {
	path := []*processedBlock{pb}
	for {
		// Stop at the common parent.
		if pb.Height <= blockHeight(tx) && getPath(tx, pb.Height) == pb.Block.ID() {
			break
		}
		pb = getBlockMap(tx, pb.Parent)
		path = append([]*processedBlock{pb}, path...) // prepend
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
	_ = cs.db.Update(func(tx *bolt.Tx) error {
		for currentBlockID(tx) != pb.Block.ID() {
			node := currentProcessedBlock(tx)
			err := commitDiffSet(tx, node, modules.DiffRevert)
			if build.DEBUG && err != nil {
				panic(err)
			}
			revertedNodes = append(revertedNodes, node)
		}
		return nil
	})
	return revertedNodes
}

// applyUntilNode will successively apply the blocks between the consensus
// set's current path and 'pb'.
func (cs *ConsensusSet) applyUntilNode(pb *processedBlock) (appliedBlocks []*processedBlock, err error) {
	// Backtrack to the common parent of 'bn' and current path and then apply the new nodes.
	var newPath []*processedBlock
	_ = cs.db.Update(func(tx *bolt.Tx) error {
		newPath = backtrackToCurrentPath(tx, pb)
		return nil
	})
	for _, node := range newPath[1:] {
		// If the diffs for this node have already been generated, apply diffs
		// directly instead of generating them. This is much faster.
		if node.DiffsGenerated {
			err = cs.db.Update(func(tx *bolt.Tx) error {
				return commitDiffSet(tx, node, modules.DiffApply)
			})
			if build.DEBUG && err != nil {
				panic(err)
			}
		} else {
			err = cs.db.Update(func(tx *bolt.Tx) error {
				return generateAndApplyDiff(tx, node)
			})
			if err != nil {
				// Mark the block as invalid.
				cs.dosBlocks[node.Block.ID()] = struct{}{}
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
	var commonParent *processedBlock
	_ = cs.db.Update(func(tx *bolt.Tx) error {
		commonParent = backtrackToCurrentPath(tx, newNode)[0]
		return nil
	})
	revertedNodes = cs.revertToNode(commonParent)

	// fast-forward to newNode
	appliedNodes, err = cs.applyUntilNode(newNode)
	if err == nil {
		return revertedNodes, appliedNodes, nil
	}

	// restore old path
	//
	// TODO: Won't be needed.
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
