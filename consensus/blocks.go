package consensus

import (
	"errors"
	"math/big"
	"sort"
	"time"
)

// TODO: Find a better place for this. SurpassThreshold isn't really a
// consensus rule, it can be modified. That tells me it should be in siad but
// deciding when to fork is pretty fundamental to the AcceptBlock code.
var SurpassThreshold = big.NewRat(5, 100)

// Exported Errors
var (
	BlockKnownErr    = errors.New("block exists in block map.")
	FutureBlockErr   = errors.New("timestamp too far in future, will try again later.")
	KnownOrphanErr   = errors.New("block is a known orphan")
	UnknownOrphanErr = errors.New("block is an unknown orphan")
)

// EarliestLegalChildTimestamp() returns the earliest a timestamp can be for the child
// of a BlockNode to be legal.
func (bn *BlockNode) earliestLegalChildTimestamp() Timestamp {
	var intTimestamps []int
	for _, timestamp := range bn.RecentTimestamps {
		intTimestamps = append(intTimestamps, int(timestamp))
	}
	sort.Ints(intTimestamps)
	return Timestamp(intTimestamps[5])
}

// State.checkMaps() looks through the maps known to the state and sees if the
// block id has been cached anywhere.
func (s *State) checkMaps(b *Block) (parentBlockNode *BlockNode, err error) {
	// See if the block is a known invalid block.
	_, exists := s.badBlocks[b.ID()]
	if exists {
		err = errors.New("block is known to be invalid")
		return
	}

	// See if the block is a known valid block.
	_, exists = s.blockMap[b.ID()]
	if exists {
		err = BlockKnownErr
		return
	}

	// See if the block's parent is known.
	parentBlockNode, exists = s.blockMap[b.ParentBlockID]
	if !exists {
		// See if the block is a known orphan block.
		orphansOfParent, exists := s.orphanMap[b.ParentBlockID]
		if !exists {
			// Make the map for the parent - parent has not been seen before.
			s.orphanMap[b.ParentBlockID] = make(map[BlockID]*Block)
		} else {
			_, exists = orphansOfParent[b.ID()]
			if exists {
				err = KnownOrphanErr
				return
			}
		}
		// Add the block to the list of known orphans.
		s.orphanMap[b.ParentBlockID][b.ID()] = b

		err = UnknownOrphanErr
		return
	}

	return
}

// State.validateHaeader() returns err = nil if the header information in the
// block (everything except the transactions) is valid, and returns an error
// explaining why validation failed if the header is invalid.
func (s *State) validateHeader(parent *BlockNode, b *Block) (err error) {
	// Check the id meets the target.
	if !b.CheckTarget(parent.Target) {
		err = errors.New("block does not meet target")
		return
	}

	// Check that the block is not too far in the future.
	// TODO: sleep for 30 seconds at a time
	skew := b.Timestamp - Timestamp(time.Now().Unix())
	if skew > FutureThreshold {
		go func(skew Timestamp, b Block) {
			time.Sleep(time.Duration(skew-FutureThreshold) * time.Second)
			s.Lock()
			s.AcceptBlock(b)
			s.Unlock()
		}(skew, *b)
		err = FutureBlockErr
		return
	}

	// If timestamp is too far in the past, reject and put in bad blocks.
	if parent.earliestLegalChildTimestamp() > b.Timestamp {
		s.badBlocks[b.ID()] = struct{}{}
		err = errors.New("timestamp invalid for being in the past")
		return
	}

	// Check that the transaction merkle root matches the transactions
	// included into the block.
	if b.MerkleRoot != b.TransactionMerkleRoot() {
		s.badBlocks[b.ID()] = struct{}{}
		err = errors.New("merkle root does not match transactions sent.")
		return
	}

	return
}

// State.childTarget() calculates the proper target of a child node given the
// parent node, and copies the target into the child node.
func (s *State) childTarget(parentNode *BlockNode, newNode *BlockNode) Target {
	var timePassed, expectedTimePassed Timestamp
	if newNode.Height < TargetWindow {
		timePassed = newNode.Block.Timestamp - s.blockRoot.Block.Timestamp
		expectedTimePassed = BlockFrequency * Timestamp(newNode.Height)
	} else {
		// THIS CODE ASSUMES THAT THE BLOCK AT HEIGHT
		// NEWNODE.HEIGHT-TARGETWINDOW IS THE SAME FOR BOTH THE NEW NODE AND
		// THE CURRENT FORK. IN GENERAL THIS IS A PRETTY SAFE ASSUMPTION AS ITS
		// LOOKING BACKWARDS BY 5000 BLOCKS. BUT WE SHOULD PROBABLY IMPLEMENT
		// SOMETHING THATS FULLY SAFE REGARDLESS.
		adjustmentBlock, err := s.BlockAtHeight(newNode.Height - TargetWindow)
		if err != nil {
			panic(err)
		}
		timePassed = newNode.Block.Timestamp - adjustmentBlock.Timestamp
		expectedTimePassed = BlockFrequency * Timestamp(TargetWindow)
	}

	// Adjustment = timePassed / expectedTimePassed.
	targetAdjustment := big.NewRat(int64(timePassed), int64(expectedTimePassed))

	// Enforce a maximum targetAdjustment
	if targetAdjustment.Cmp(MaxAdjustmentUp) == 1 {
		targetAdjustment = MaxAdjustmentUp
	} else if targetAdjustment.Cmp(MaxAdjustmentDown) == -1 {
		targetAdjustment = MaxAdjustmentDown
	}

	newTarget := new(big.Rat).Mul(parentNode.Target.Rat(), targetAdjustment)
	return RatToTarget(newTarget)
}

// State.childDepth() returns the cumulative weight of all the blocks leading
// up to and including the child block.
// childDepth := (1/parentTarget + 1/parentDepth)^-1
func (s *State) childDepth(parentNode *BlockNode) (depth Target) {
	cumulativeDifficulty := new(big.Rat).Add(parentNode.Target.Inverse(), parentNode.Depth.Inverse())
	return RatToTarget(new(big.Rat).Inv(cumulativeDifficulty))
}

// State.addBlockToTree() takes a block and a parent node, and adds a child
// node to the parent containing the block. No validation is done.
func (s *State) addBlockToTree(parentNode *BlockNode, b *Block) (newNode *BlockNode) {
	// Create the child node.
	newNode = new(BlockNode)
	newNode.Block = b
	newNode.Height = parentNode.Height + 1

	// Copy over the timestamps.
	copy(newNode.RecentTimestamps[:], parentNode.RecentTimestamps[1:])
	newNode.RecentTimestamps[10] = b.Timestamp

	// Calculate target and depth.
	newNode.Target = s.childTarget(parentNode, newNode)
	newNode.Depth = s.childDepth(parentNode)

	// Add the node to the block map and the list of its parents children.
	s.blockMap[b.ID()] = newNode
	parentNode.Children = append(parentNode.Children, newNode)

	return
}

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
func (s *State) rewindABlock() {
	// Remove the output for the miner subsidy.
	delete(s.unspentOutputs, s.CurrentBlock().SubsidyID())

	// As new transaction types get added, perform inverse maintenance here.

	// Perform inverse contract maintenance.
	s.inverseContractMaintenance()

	// Reverse each transaction in the block, in reverse order from how
	// they appear in the block.
	for i := len(s.CurrentBlock().Transactions) - 1; i >= 0; i-- {
		s.reverseTransaction(s.CurrentBlock().Transactions[i])
	}

	// Update the CurrentBlock and CurrentPath variables of the longest fork.
	delete(s.currentPath, s.Height())
	s.currentBlockID = s.CurrentBlock().ParentBlockID
}

// s.integrateBlock() will verify the block and then integrate it into the
// consensus state.
func (s *State) integrateBlock(b *Block) (err error) {
	var appliedTransactions []Transaction
	minerSubsidy := Currency(0)
	for _, txn := range b.Transactions {
		err = s.ValidTransaction(txn)
		if err != nil {
			break
		}

		// Apply the transaction to the ConsensusState, adding it to the list of applied transactions.
		s.applyTransaction(txn)
		appliedTransactions = append(appliedTransactions, txn)

		// Add the miner fees to the miner subsidy.
		for _, fee := range txn.MinerFees {
			minerSubsidy += fee
		}
	}

	if err != nil {
		// Rewind transactions added.
		for i := len(appliedTransactions) - 1; i >= 0; i-- {
			s.reverseTransaction(appliedTransactions[i])
		}
		return
	}

	// Perform maintanence on all open contracts.
	s.contractMaintenance()

	// As new transaction types get added, perform maintenance here.

	// Update the current block and current path variables of the longest fork.
	s.currentBlockID = b.ID()
	s.currentPath[s.blockMap[b.ID()].Height] = b.ID()

	// Add coin inflation to the miner subsidy.
	minerSubsidy += CalculateCoinbase(s.Height())

	// Add output contianing miner fees + block subsidy.
	minerSubsidyOutput := Output{
		Value:     minerSubsidy,
		SpendHash: b.MinerAddress,
	}
	s.unspentOutputs[b.SubsidyID()] = minerSubsidyOutput

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
func (s *State) forkBlockchain(newNode *BlockNode) (rewoundBlocks []BlockID, appliedBlocks []BlockID, err error) {
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

	// Remove blocks from the ConsensusState until we get to the
	// same parent that we are forking from.
	for s.currentBlockID != currentNode.Block.ID() {
		rewoundBlocks = append(rewoundBlocks, s.currentBlockID)
		s.rewindABlock()
	}

	// Validate each block in the parent history in order, updating
	// the state as we go.  If at some point a block doesn't
	// verify, you get to walk all the way backwards and forwards
	// again.
	validatedBlocks := 0
	for i := len(parentHistory) - 1; i >= 0; i-- {
		appliedBlock := s.blockMap[parentHistory[i]].Block
		appliedBlocks = append(appliedBlocks, appliedBlock.ID())
		err = s.integrateBlock(appliedBlock)
		if err != nil {
			// Add the whole tree of blocks to BadBlocks,
			// deleting them from BlockMap
			s.invalidateNode(s.blockMap[parentHistory[i]])

			// Rewind the validated blocks
			for i := 0; i < validatedBlocks; i++ {
				s.rewindABlock()
			}
			appliedBlocks = nil // Reset applied blocks to nil since nothing in sum was applied.

			// Integrate the rewound blocks
			for i := len(rewoundBlocks) - 1; i >= 0; i-- {
				err = s.integrateBlock(s.blockMap[rewoundBlocks[i]].Block)
				if err != nil {
					panic("Once-validated blocks are no longer validating - state logic has mistakes.")
				}
			}
			rewoundBlocks = nil // Reset rewoundBlocks to nil since nothing in sum was rewound.

			break
		}
		validatedBlocks += 1
	}

	// Update the transaction pool to remove any transactions that have
	// invalidated on account of invalidated storage proofs.
	s.cleanTransactionPool()

	return
}

// State.AcceptBlock() will add blocks to the state, forking the blockchain if
// they are on a fork that is heavier than the current fork.
func (s *State) AcceptBlock(b Block) (rewoundBlocks []BlockID, appliedBlocks []BlockID, err error) {
	// Check the maps in the state to see if the block is already known.
	parentBlockNode, err := s.checkMaps(&b)
	if err != nil {
		return
	}

	// Check that the header of the block is valid.
	err = s.validateHeader(parentBlockNode, &b)
	if err != nil {
		return
	}

	newBlockNode := s.addBlockToTree(parentBlockNode, &b)

	// If the new node is 5% heavier than the current node, switch to the new fork.
	if s.heavierFork(newBlockNode) {
		rewoundBlocks, appliedBlocks, err = s.forkBlockchain(newBlockNode)
		if err != nil {
			return
		}
	}

	// Perform a sanity check if debug flag is set.
	if DEBUG {
		s.CurrentPathCheck()
	}

	return
}
