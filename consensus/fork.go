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
func (s *State) backtrackToBlockchain(node *BlockNode) (nodes []*BlockNode) {
	nodes = append(nodes, node)
	currentChainID := s.currentPath[node.Height]
	for currentChainID != node.Block.ID() {
		node = node.Parent
		currentChainID = s.currentPath[node.Height]
		nodes = append(nodes, node)
	}
	return
}

func (s *State) invertRecentBlock() {
	bn := s.currentBlockNode()
	for _, od := range bn.OutputDiffs {
		s.invertOutputDiff(od)
	}
	for _, cd := range bn.ContractDiffs {
		s.invertContractDiff(cd)
	}
}

// rewindBlockchain will rewind blocks until the common parent is the highest
// block.
func (s *State) rewindBlockchain(commonParent *BlockNode) (rewoundNodes []*BlockNode) {
	// Sanity check to make sure that commonParent is in the currentPath.
	if DEBUG {
		if commonParent.Block.ID() != s.currentPath[commonParent.Height] {
			panic("bad use of rewindBlockchain")
		}
	}

	// Remove blocks from the ConsensusState until we get to the
	// same parent that we are forking from.
	for s.currentBlockID != commonParent.Block.ID() {
		rewoundNodes = append(rewoundNodes, s.currentBlockNode())
		s.invertRecentBlock()
	}
	return
}

// s.integrateBlock() will verify the block and then integrate it into the
// consensus state.
func (s *State) generateAndApplyDiff(bn *BlockNode) (err error) {
	if DEBUG {
		if bn.DiffsGenerated {
			panic("misuse of generateAndApplyDiff")
		}
	}
	bn.BlockDiff.CatalystBlock = bn.Block.ID()

	minerSubsidy := Currency(0)
	for _, txn := range bn.Block.Transactions {
		err = s.validTransaction(txn)
		if err != nil {
			break
		}

		// Apply the transaction to the ConsensusState, adding it to the list of applied transactions.
		outputDiffs, contractDiffs := s.applyTransaction(txn)
		bn.BlockDiff.OutputDiffs = append(bn.BlockDiff.OutputDiffs, outputDiffs...)
		bn.BlockDiff.ContractDiffs = append(bn.BlockDiff.ContractDiffs, contractDiffs...)

		// Add the miner fees to the miner subsidy.
		for _, fee := range txn.MinerFees {
			minerSubsidy += fee
		}
	}
	if err != nil {
		invertBlockNode(bn)
		return
	}

	// Perform maintanence on all open contracts.
	s.applyContractMaintenance(bn)

	// Add coin inflation to the miner subsidy.
	minerSubsidy += CalculateCoinbase(s.height())

	// Add output contianing miner fees + block subsidy.
	//
	// TODO: Add this to the list of future miner subsidies
	minerSubsidyOutput := Output{
		Value:     minerSubsidy,
		SpendHash: b.MinerAddress,
	}
	subsidyDiff := OutputDiff{
		New:    true,
		ID:     b.SubsidyID(),
		Output: minerSubsidyOutput,
	}
	s.unspentOutputs[b.SubsidyID()] = minerSubsidyOutput
	bn.BlockDiff.OutputDiffs = append(bn.BlockDiff.OutputDiffs, subsidyDiff)

	// Update the current block and current path variables of the longest fork.
	height := s.blockMap[b.ID()].Height
	s.currentBlockID = b.ID()
	s.currentPath[height] = b.ID()

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

// forkBlockchain() will go from the current block over to a block on a
// different fork, rewinding and integrating blocks as needed. forkBlockchain()
// will return an error if any of the blocks in the new fork are invalid.
func (s *State) forkBlockchain(newNode *BlockNode) (rewoundBlocks []Block, appliedBlocks []Block, outputDiffs []OutputDiff, err error) {
	// Get the state hash before attempting a fork.
	var stateHash hash.Hash
	if DEBUG {
		stateHash = s.stateHash()
	}

	// Get the list of blocks tracing from the new node to the blockchain.
	backtrackNodes := s.backtrackToBlockchain(newNode)

	// Rewind the blockchain to the common parent.
	commonParent := backtrackNodes[len(backtrackNodes)-1]
	rewoundNodes := s.rewindBlockchain(commonParent)

	// Validate each block in the parent history in order, updating
	// the state as we go.  If at some point a block doesn't
	// verify, you get to walk all the way backwards and forwards
	// again.
	var appliedNodes []*BlockNode
	for i := len(backtrackNodes) - 1; i >= 0; i-- {
		appliedNodes = append(appliedNodes, backtrackNodes[i])
		err = s.generateAndApplyDiff(backtrackNodes[i])
		if err != nil {
			// Invalidate and delete all of the nodes after the bad block.
			s.invalidateNode(parentHistory[i])

			// Rewind the validated blocks
			for i := 0; i < len(appliedNodes); i++ {
				s.invertRecentBlock()
			}

			// Integrate the rewound blocks
			for i := len(rewoundBlocks) - 1; i >= 0; i-- {
				_, err = s.integrateBlock(rewoundBlocks[i], &BlockDiff{}) // this diff is not used, because the state has not changed. TODO: change how reapply works.
				if err != nil {
					panic("Once-validated blocks are no longer validating - state logic has mistakes.")
				}
			}

			// Reset diffs to nil since nothing in sum was changed.
			appliedBlocks = nil
			rewoundBlocks = nil
			outputDiffs = nil
			bd = BlockDiff{}

			// Check that the state hash is the same as before forking and then returning.
			if DEBUG {
				if stateHash != s.stateHash() {
					panic("state hash does not match after an unsuccessful fork attempt")
				}
			}

			break
		}
		cc.AppliedBlocks = append(cc.AppliedBlocks, bd)
		s.blockMap[parentHistory[i]].BlockDiff = bd
		// TODO: Add the block diff to the block node, for retrieval during inversion.
		validatedBlocks += 1
		outputDiffs = append(outputDiffs, diffSet...)
	}

	// Update the transaction pool to remove any transactions that have
	// invalidated on account of invalidated storage proofs.
	s.cleanTransactionPool()

	// Notify all subscribers of the changes.
	if appliedBlocks != nil {
		s.notifySubscribers(cc)
	}

	return
}
