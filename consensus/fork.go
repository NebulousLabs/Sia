package consensus

import (
	"github.com/NebulousLabs/Sia/crypto"
)

// backtrackToBlockchain returns a list of nodes that go from the current node
// to the first common parent. The common parent will be the final node in the
// slice.
func (s *State) backtrackToBlockchain(bn *blockNode) (nodes []*blockNode) {
	nodes = append(nodes, bn)
	for s.currentPath[bn.height] != bn.block.ID() {
		bn = bn.parent
		nodes = append(nodes, bn)

		// Sanity check - all block nodes should have a parent except the
		// genesis block. This loop should break before reaching the genesis
		// block.
		if bn == nil {
			if DEBUG {
				panic("backtrack hit a nil node?")
			}
			return
		}
	}
	return
}

// rewindToNode will rewind blocks until `bn` is the highest block, returning
// the list of nodes that got rewound.
func (s *State) rewindToNode(bn *blockNode) (rewoundNodes []*blockNode) {
	// Sanity check - make sure that bn is in the currentPath.
	if DEBUG {
		if bn.block.ID() != s.currentPath[bn.height] {
			panic("bad use of rewindToNode")
		}
	}

	// Remove blocks from the ConsensusState until we get to the
	// same parent that we are forking from.
	for s.currentBlockID != bn.block.ID() {
		bn := s.currentBlockNode()
		rewoundNodes = append(rewoundNodes, bn)
		direction := false // direction is set to false because the node is being removed.
		s.applyDiffSet(bn, direction)
	}
	return
}

// invalidateNode recursively deletes all the generational children of a block
// and puts them all on the bad blocks list.
func (s *State) invalidateNode(node *blockNode) {
	for i := range node.children {
		s.invalidateNode(node.children[i])
	}

	delete(s.blockMap, node.block.ID())
	s.badBlocks[node.block.ID()] = struct{}{}
}

// forkBlockchain will take the consensus of the State from whatever node it's
// currently on to the node presented. An error will be returned if any of the
// blocks that get applied in the transition are found to be invalid. If an
// error is returned, forkBlockchain will bring the consensus variables back to
// how they were before the call was made.
func (s *State) forkBlockchain(newNode *blockNode) (err error) {
	// Get the state hash before attempting a fork.
	var stateHash crypto.Hash
	if DEBUG {
		stateHash = s.stateHash()
	}

	// Get the list of blocks tracing from the new node to the blockchain, then
	// rewind to the common parent.
	backtrackNodes := s.backtrackToBlockchain(newNode)
	commonParent := backtrackNodes[len(backtrackNodes)-1]
	rewoundNodes := s.rewindToNode(commonParent)

	// Update the consensus to include all of the block nodes that go from the
	// common parent to `newNode`. If any of the blocks are invalid, reverse
	// all of the changes and switch back to the original block.
	var appliedNodes []*blockNode
	for i := len(backtrackNodes) - 2; i >= 0; i-- {
		bn := backtrackNodes[i]
		appliedNodes = append(appliedNodes, bn)

		// If the diffs for this node have already been generated, apply diffs
		// directly instead of generating them. This is much faster.
		if bn.diffsGenerated {
			direction := true // the blockNode is being applied, direction is set to true.
			s.applyDiffSet(bn, direction)
			continue
		}

		// If the diffs have not been generated, call generateAndApplyDiff.
		// This call will fail if the block is somehow invalid. If the call
		// fails, all of the applied blocks will be reversed, and all of the
		// rewound blocks will be reapplied, restoring the consensus of the
		// State to its original condition.
		err = s.generateAndApplyDiff(bn)
		if err != nil {
			// Mark the invalid block, then rewind all the new blocks and
			// reapply all of the rewound blocks.
			s.invalidateNode(bn)
			s.rewindToNode(commonParent)
			for i := len(rewoundNodes) - 1; i >= 0; i-- {
				direction := true // the blockNode is being applied, direction is set to true.
				s.applyDiffSet(rewoundNodes[i], direction)
			}

			// Check that the state hash is the same as before forking and then returning.
			if DEBUG {
				if stateHash != s.stateHash() {
					panic("state hash does not match after an unsuccessful fork attempt")
				}
			}

			return
		}
	}

	return
}
