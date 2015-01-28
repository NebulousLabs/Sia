package consensus

import (
	"math/big"

	"github.com/NebulousLabs/Sia/hash"
)

// State.heavierFork() returns true if the input node is 5% heavier than the
// current node of the ConsensusState.
func (s *State) heavierFork(newNode *BlockNode) bool {
	threshold := new(big.Rat).Mul(s.currentBlockWeight(), SurpassThreshold)
	currentCumDiff := s.depth().Inverse()
	requiredCumDiff := new(big.Rat).Add(currentCumDiff, threshold)
	newNodeCumDiff := newNode.Depth.Inverse()
	return newNodeCumDiff.Cmp(requiredCumDiff) == 1
}

// backtrackToBlockchain returns a list of nodes that go from the current node
// to the first parent that is in the current blockchain.
func (s *State) backtrackToBlockchain(bn *BlockNode) (nodes []*BlockNode) {
	nodes = append(nodes, bn)
	for s.currentPath[bn.Height] != bn.Block.ID() {
		bn = bn.Parent
		nodes = append(nodes, bn)
	}
	return
}

func (s *State) invertRecentBlock() {
	bn := s.currentBlockNode()

	// Invert all of the diffs.
	direction := false // blockchain is inverting, set direction flag to false.
	for _, od := range bn.OutputDiffs {
		s.commitOutputDiff(od, direction)
	}
	for _, cd := range bn.ContractDiffs {
		s.commitContractDiff(cd, direction)
	}

	// Update the current path and currentBlockID
	delete(s.currentPath, bn.Height)
	s.currentBlockID = bn.Parent.Block.ID()
}

// rewindToNode will rewind blocks until `bn` is the highest block.
func (s *State) rewindToNode(bn *BlockNode) (rewoundNodes []*BlockNode) {
	// Sanity check  - make sure that bn is in the currentPath.
	if DEBUG {
		if bn.Block.ID() != s.currentPath[bn.Height] {
			panic("bad use of rewindToNode")
		}
	}

	// Remove blocks from the ConsensusState until we get to the
	// same parent that we are forking from.
	for s.currentBlockID != bn.Block.ID() {
		rewoundNodes = append(rewoundNodes, s.currentBlockNode())
		s.invertRecentBlock()
	}
	return
}

// s.integrateBlock() will verify the block and then integrate it into the
// consensus state.
func (s *State) generateAndApplyDiff(bn *BlockNode) (err error) {
	// Sanity check - generate should only be called if the diffs have not yet
	// been generated.
	if DEBUG {
		if bn.DiffsGenerated {
			panic("misuse of generateAndApplyDiff")
		}
	}

	// Update the current block and current path.
	s.currentBlockID = bn.Block.ID()
	s.currentPath[bn.Height] = bn.Block.ID()

	minerSubsidy := CalculateCoinbase(s.height())
	for _, txn := range bn.Block.Transactions {
		err = s.validTransaction(txn)
		if err != nil {
			break
		}

		// Apply the transaction to the ConsensusState, adding it to the list of applied transactions.
		outputDiffs, contractDiffs := s.applyTransaction(txn)
		bn.OutputDiffs = append(bn.OutputDiffs, outputDiffs...)
		bn.ContractDiffs = append(bn.ContractDiffs, contractDiffs...)

		// Add the miner fees to the miner subsidy.
		for _, fee := range txn.MinerFees {
			minerSubsidy += fee
		}
	}
	if err != nil {
		s.invertRecentBlock()
		return
	}

	// Perform maintanence on all open contracts.
	outputDiffs, contractDiffs := s.applyContractMaintenance()
	bn.OutputDiffs = append(bn.OutputDiffs, outputDiffs...)
	bn.ContractDiffs = append(bn.ContractDiffs, contractDiffs...)

	// Add output contianing miner subsidy.
	subsidyOutput := Output{
		Value:     minerSubsidy,
		SpendHash: bn.Block.MinerAddress,
	}
	subsidyDiff := OutputDiff{
		New:    true,
		ID:     bn.Block.SubsidyID(),
		Output: subsidyOutput,
	}
	s.unspentOutputs[bn.Block.SubsidyID()] = subsidyOutput
	bn.OutputDiffs = append(bn.OutputDiffs, subsidyDiff)

	return
}

// invalidateNode() is a recursive function that deletes all of the
// children of a block and puts them on the bad blocks list.
func (s *State) invalidateNode(node *BlockNode) {
	for i := range node.Children {
		s.invalidateNode(node.Children[i])
	}

	delete(s.blockMap, node.Block.ID())
	s.badBlocks[node.Block.ID()] = struct{}{}
}

func (s *State) applyBlockNode(bn *BlockNode) {
	// Sanity check - current node must be the input node's parent.
	if DEBUG {
		if bn.Parent.Block.ID() != s.currentBlockID {
			panic("applying a block node when it's not a valid successor")
		}
	}

	// Update current id and current path.
	s.currentBlockID = bn.Block.ID()
	s.currentPath[bn.Height] = bn.Block.ID()

	// Apply all of the diffs.
	direction := true // blockchain is going forward, set direction flag to true.
	for _, od := range bn.OutputDiffs {
		s.commitOutputDiff(od, direction)
	}
	for _, cd := range bn.ContractDiffs {
		s.commitContractDiff(cd, direction)
	}
}

// forkBlockchain() will go from the current block over to a block on a
// different fork, rewinding and integrating blocks as needed. forkBlockchain()
// will return an error if any of the blocks in the new fork are invalid.
func (s *State) forkBlockchain(newNode *BlockNode) (err error) {
	// Get the state hash before attempting a fork.
	var stateHash hash.Hash
	if DEBUG {
		stateHash = s.stateHash()
	}

	// Get the list of blocks tracing from the new node to the blockchain.
	backtrackNodes := s.backtrackToBlockchain(newNode)

	// Rewind the blockchain to the common parent.
	commonParent := backtrackNodes[len(backtrackNodes)-1]
	rewoundNodes := s.rewindToNode(commonParent)

	// Validate each block in the parent history in order, updating
	// the state as we go.  If at some point a block doesn't
	// verify, you get to walk all the way backwards and forwards
	// again.
	var appliedNodes []*BlockNode
	for i := len(backtrackNodes) - 1; i >= 0; i-- {
		appliedNodes = append(appliedNodes, backtrackNodes[i])

		// If the diffs for this node have already been generated, apply them
		// directly instead of generating them.
		if backtrackNodes[i].DiffsGenerated {
			for _, outputDiff := range backtrackNodes[i].OutputDiffs {
				s.commitOutputDiff(outputDiff, true)
			}
			for _, contractDiff := range backtrackNodes[i].ContractDiffs {
				s.commitContractDiff(contractDiff, true)
			}
			continue
		}

		// If the diffs have not been generated, call generateAndApply.
		err = s.generateAndApplyDiff(backtrackNodes[i])
		if err != nil {
			// Invalidate and delete all of the nodes after the bad block.
			s.invalidateNode(backtrackNodes[i])

			// Rewind the validated blocks
			for j := 0; j < len(appliedNodes)-1; j++ {
				s.invertRecentBlock()
			}

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

	// Sanity check - make sure the currentPath makes sense.
	if DEBUG {
		s.currentPathCheck()
	}

	return
}
