package consensus

import (
	"math/big"

	"github.com/NebulousLabs/Sia/hash"
)

// State.heavierFork() returns true if the input node is 5% heavier than the
// current node of the ConsensusState.
func (s *State) heavierFork(newNode *BlockNode) bool {
	threshold := new(big.Rat).Mul(s.CurrentBlockWeight(), SurpassThreshold)
	currentCumDiff := s.Depth().Inverse()
	requiredCumDiff := new(big.Rat).Add(currentCumDiff, threshold)
	newNodeCumDiff := newNode.Depth.Inverse()
	return newNodeCumDiff.Cmp(requiredCumDiff) == 1
}

// State.rewindABlock() removes the most recent block from the ConsensusState,
// making the ConsensusState as though the block had never been integrated.
func (s *State) invertRecentBlock() (diffs []OutputDiff) {
	// Remove the output for the miner subsidy.
	//
	// TODO: Update this for incentive stuff - miner doesn't get subsidy until
	// 2000 or 5000 or 10000 blocks later.
	subsidyID := s.CurrentBlock().SubsidyID()
	subsidy, err := s.Output(subsidyID)
	if err != nil {
		panic(err)
	}
	diff := OutputDiff{New: false, ID: subsidyID, Output: subsidy}
	diffs = append(diffs, diff)
	delete(s.unspentOutputs, subsidyID)

	// Perform inverse contract maintenance.
	diffSet := s.invertContractMaintenance()
	diffs = append(diffs, diffSet...)

	// Reverse each transaction in the block, in reverse order from how
	// they appear in the block.
	for i := len(s.CurrentBlock().Transactions) - 1; i >= 0; i-- {
		diffSet := s.invertTransaction(s.CurrentBlock().Transactions[i])
		diffs = append(diffs, diffSet...)
	}

	// Update the CurrentBlock and CurrentPath variables of the longest fork.
	delete(s.currentPath, s.Height())
	s.currentBlockID = s.CurrentBlock().ParentBlockID
	return
}

// s.integrateBlock() will verify the block and then integrate it into the
// consensus state.
func (s *State) integrateBlock(b Block, bd *BlockDiff) (diffs []OutputDiff, err error) {
	bd.CatalystBlock = b.ID()

	var appliedTransactions []Transaction
	minerSubsidy := Currency(0)
	for _, txn := range b.Transactions {
		err = s.ValidTransaction(txn)
		if err != nil {
			break
		}

		// Apply the transaction to the ConsensusState, adding it to the list of applied transactions.
		transactionDiff := s.applyTransaction(txn)
		appliedTransactions = append(appliedTransactions, txn)
		diffs = append(diffs, transactionDiff.OutputDiffs...)
		bd.TransactionDiffs = append(bd.TransactionDiffs, transactionDiff)

		// Add the miner fees to the miner subsidy.
		for _, fee := range txn.MinerFees {
			minerSubsidy += fee
		}
	}

	if err != nil {
		// Rewind transactions added.
		for i := len(appliedTransactions) - 1; i >= 0; i-- {
			s.invertTransaction(appliedTransactions[i])
		}
		return
	}

	// Perform maintanence on all open contracts.
	diffSet := s.applyContractMaintenance(&bd.BlockChanges)
	diffs = append(diffs, diffSet...)

	// Update the current block and current path variables of the longest fork.
	height := s.blockMap[b.ID()].Height
	s.currentBlockID = b.ID()
	s.currentPath[height] = b.ID()

	// Add coin inflation to the miner subsidy.
	minerSubsidy += CalculateCoinbase(s.Height())

	// Add output contianing miner fees + block subsidy.
	//
	// TODO: Add this to the list of future miner subsidies
	minerSubsidyOutput := Output{
		Value:     minerSubsidy,
		SpendHash: b.MinerAddress,
	}
	s.unspentOutputs[b.SubsidyID()] = minerSubsidyOutput
	diff := OutputDiff{New: true, ID: b.SubsidyID(), Output: minerSubsidyOutput}
	diffs = append(diffs, diff)
	bd.BlockChanges.OutputDiffs = append(bd.BlockChanges.OutputDiffs, diffs...)

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
	// Create a block diff for use when calling integrateBlock.
	//
	// TODO: Move this somewhere else.
	var cc ConsensusChange

	// Find the common parent between the new fork and the current
	// fork, keeping track of which path is taken through the
	// children of the parents so that we can re-trace as we
	// validate the blocks.
	currentNode := newNode
	value := s.currentPath[currentNode.Height]
	var parentHistory []BlockID
	for value != currentNode.Block.ID() {
		parentHistory = append(parentHistory, currentNode.Block.ID())
		currentNode = s.blockMap[currentNode.Block.ParentBlockID]
		value = s.currentPath[currentNode.Height]
	}

	// Get the state hash before attempting a fork.
	var stateHash hash.Hash
	if DEBUG {
		stateHash = s.StateHash()
	}

	// Remove blocks from the ConsensusState until we get to the
	// same parent that we are forking from.
	for s.currentBlockID != currentNode.Block.ID() {
		rewoundBlocks = append(rewoundBlocks, s.CurrentBlock())
		cc.InvertedBlocks = append(cc.InvertedBlocks, s.currentBlockNode().BlockDiff)
		outputDiffs = append(outputDiffs, s.invertRecentBlock()...)
	}

	// Validate each block in the parent history in order, updating
	// the state as we go.  If at some point a block doesn't
	// verify, you get to walk all the way backwards and forwards
	// again.
	validatedBlocks := 0
	for i := len(parentHistory) - 1; i >= 0; i-- {
		appliedBlock := s.blockMap[parentHistory[i]].Block
		appliedBlocks = append(appliedBlocks, appliedBlock)
		var bd BlockDiff
		diffSet, err := s.integrateBlock(appliedBlock, &bd)
		if err != nil {
			// Add the whole tree of blocks to BadBlocks,
			// deleting them from BlockMap
			s.invalidateNode(s.blockMap[parentHistory[i]])

			// Rewind the validated blocks
			for i := 0; i < validatedBlocks; i++ {
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
				if stateHash != s.StateHash() {
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
