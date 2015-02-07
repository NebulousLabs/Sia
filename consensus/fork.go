package consensus

import (
	"math/big"

	"github.com/NebulousLabs/Sia/crypto"
)

// A non-consensus rule that dictates how much heavier a competing chain has to
// be before the node will switch to mining on that chain. The percent refers
// to the percent of the weight of the most recent block on the winning chain,
// not the weight of the entire chain.
//
// This rule is in place because the difficulty gets updated every block, and
// that means that of two competing blocks, one could be very slightly heavier.
// The slightly heavier one should not be switched to if it was not seen first,
// because the amount of extra weight in the chain is inconsequential. The
// maximum difficulty shift will prevent people from manipulating timestamps
// enough to produce a block that is substantially heavier.
var (
	SurpassThreshold = big.NewRat(50, 100)
)

// heavierFork compares the depth of `newNode` to the depth of the current
// node, and returns true if `newNode` is sufficiently heavier, where
// sufficiently is defined by the weight of the current block times
// `SurpassThreshold`.
func (s *State) heavierFork(newNode *blockNode) bool {
	threshold := new(big.Rat).Mul(s.currentBlockWeight(), SurpassThreshold)
	currentCumDiff := s.depth().Inverse()
	requiredCumDiff := new(big.Rat).Add(currentCumDiff, threshold)
	newNodeCumDiff := newNode.depth.Inverse()
	return newNodeCumDiff.Cmp(requiredCumDiff) == 1
}

// backtrackToBlockchain returns a list of nodes that go from the current node
// to the first common parent. The common parent will be the final node in the
// slice.
func (s *State) backtrackToBlockchain(bn *blockNode) (nodes []*blockNode) {
	nodes = append(nodes, bn)
	for s.currentPath[bn.height] != bn.block.ID() {
		bn = bn.parent
		nodes = append(nodes, bn)

		// Sanity check - all block nodes should have a parent except the
		// geensis block. This loop should break when the gensis node is
		// appended at latest.
		if bn == nil {
			if DEBUG {
				panic("backtrack hit a nil node?")
			} else {
				return
			}
		}
	}
	return
}

// invertRecentBlock will pull the current block out of the consensus set,
// reversing all of the diffs and deleting it from the currentPath.
func (s *State) invertRecentBlock() {
	bn := s.currentBlockNode()
	direction := false // direction is set to false because the node is being removed.
	s.applyDiffSet(bn, direction)
}

// rewindToNode will rewind blocks until `bn` is the highest block.
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
		rewoundNodes = append(rewoundNodes, s.currentBlockNode())
		s.invertRecentBlock()
	}
	return
}

// applyMinerSubsidy adds all of the outputs recorded in the MinerPayouts to
// the state, and returns the corresponding set of diffs.
func (s *State) applyMinerSubsidy(bn *blockNode) {
	for i, payout := range bn.block.MinerPayouts {
		id := bn.block.MinerPayoutID(i)
		s.delayedSiacoinOutputs[s.height()][id] = payout
		bn.delayedSiacoinOutputs[id] = payout
	}
	return
}

// applyDelayedSiacoinOutputMaintenance goes through all of the outputs that
// have matured and adds them to the list of siacoinOutputs.
func (s *State) applyDelayedSiacoinOutputMaintenance(bn *blockNode) {
	for id, sco := range s.delayedSiacoinOutputs[bn.height-MaturityDelay] {
		// Sanity check - the output should not already be in the
		// siacoinOuptuts list.
		if DEBUG {
			_, exists := s.siacoinOutputs[id]
			if exists {
				panic("trying to add a delayed output when the output is already there")
			}
		}
		s.siacoinOutputs[id] = sco

		// Create and add the diff.
		scod := SiacoinOutputDiff{
			New:           true,
			ID:            id,
			SiacoinOutput: sco,
		}
		bn.siacoinOutputDiffs = append(bn.siacoinOutputDiffs, scod)
	}
}

// generateAndApplyDiff will verify the block and then integrate it into the
// consensus state.
func (s *State) generateAndApplyDiff(bn *blockNode) (err error) {
	// Sanity check - generate should only be called if the diffs have not yet
	// been generated - current node must be the input node's parent.
	if DEBUG {
		if bn.diffsGenerated {
			panic("misuse of generateAndApplyDiff")
		}
		if bn.parent.block.ID() != s.currentBlockID {
			panic("applying a block node when it's not a valid successor")
		}
	}

	// Set diffsGenerated to true.
	bn.diffsGenerated = true
	bn.siafundPoolDiff.Previous = s.siafundPool

	// Update the current block and current path.
	s.currentBlockID = bn.block.ID()
	s.currentPath[bn.height] = bn.block.ID()
	s.delayedSiacoinOutputs[s.height()] = make(map[SiacoinOutputID]SiacoinOutput)

	// Validate and apply each transaction in the block.
	for _, txn := range bn.block.Transactions {
		err = s.validTransaction(txn)
		if err != nil {
			s.invertRecentBlock()
			return
		}

		s.applyTransaction(bn, txn)
	}

	// Perform maintanence on all open contracts.
	s.applyContractMaintenance(bn)
	s.applyDelayedSiacoinOutputMaintenance(bn)
	s.applyMinerSubsidy(bn)

	// Update the siafundPoolDiff to reflect the new pool size.
	bn.siafundPoolDiff.Adjusted = s.siafundPool

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

// applyBlockNode takes a block
func (s *State) applyBlockNode(bn *blockNode) {
	// Sanity check - current node must be the input node's parent.
	if DEBUG {
		if bn.parent.block.ID() != s.currentBlockID {
			panic("applying a block node when it's not a valid successor")
		}
	}

	// Apply all of the diffs.
	direction := true // the blockNode is being applied, direction is set to true.
	s.applyDiffSet(bn, direction)
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

	// Get the list of blocks tracing from the new node to the blockchain. This
	// call will not include the common parent.
	backtrackNodes := s.backtrackToBlockchain(newNode)

	// Rewind the blockchain to the common parent.
	commonParent := backtrackNodes[len(backtrackNodes)-1]
	rewoundNodes := s.rewindToNode(commonParent)

	// Update the consensus to include all of the block nodes that go from the
	// common parent to `newNode`. If any of the blocks is invalid, reverse all
	// of the changes and switch back to the original block.
	var appliedNodes []*blockNode
	for i := len(backtrackNodes) - 2; i >= 0; i-- {
		appliedNodes = append(appliedNodes, backtrackNodes[i])

		// If the diffs for this node have already been generated, apply diffs
		// directly instead of generating them. This is much faster.
		if backtrackNodes[i].diffsGenerated {
			s.applyBlockNode(backtrackNodes[i])
			continue
		}

		// If the diffs have not been generated, call generateAndApplyDiff.
		// This call will fail if the block is somehow invalid. If the call
		// fails, all of the applied blocks will be reversed, and all of the
		// rewound blocks will be reapplied, restoring the consensus of the
		// State to its original condition.
		err = s.generateAndApplyDiff(backtrackNodes[i])
		if err != nil {
			// Invalidate and delete all of the nodes after the bad block.
			s.invalidateNode(backtrackNodes[i])

			// Rewind the validated blocks
			s.rewindToNode(commonParent)

			// Integrate the rewound nodes
			for i := len(rewoundNodes) - 1; i >= 0; i-- {
				s.applyBlockNode(rewoundNodes[i])
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
