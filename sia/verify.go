package sia

import (
	"bytes"
	"errors"
	"math/big"
	"sort"
	"time"
)

// Used to keep track of how many signatures an input has been signed by.
type InputSignatures struct {
	RemainingSignatures uint8
	PossibleKeys        []PublicKey
	UsedKeys            map[uint8]struct{}
}

// checkMaps looks through the maps known to the state and sees if the block id
// has been cached anywhere.
func (s *State) checkMaps(b *Block) (parentBlockNode *BlockNode, err error) {
	// See if the block is a known invalid block.
	_, exists := s.BadBlocks[b.ID()]
	if exists {
		err = errors.New("block is known to be invalid")
		return
	}

	// See if the block is a known valid block.
	_, exists = s.BlockMap[b.ID()]
	if exists {
		err = errors.New("Block exists in block map.")
		return
	}

	/*
		// See if the block is a known orphan.
		_, exists = s.OrphanBlocks[b.ID()]
		if exists {
			err = errors.New("Block exists in orphan list")
			return
		}
	*/

	// See if the block's parent is known.
	parentBlockNode, exists = s.BlockMap[b.ParentBlock]
	if !exists {
		// OrphanBlocks[b.ID()] = b
		err = errors.New("Block is an orphan")
		return
	}

	return
}

// Returns true if timestamp is valid, and if target value is reached.
func (s *State) validateHeader(parent *BlockNode, b *Block) (err error) {
	// Check that the block is not too far in the future.
	skew := b.Timestamp - Timestamp(time.Now().Unix())
	if skew > FutureThreshold {
		// Do something so that you will return to considering this
		// block once it's no longer too far in the future.
		err = errors.New("timestamp too far in future")
		return
	}

	// If timestamp is too far in the past, reject and put in bad blocks.
	var intTimestamps []int
	for _, timestamp := range parent.RecentTimestamps {
		intTimestamps = append(intTimestamps, int(timestamp))
	}
	sort.Ints(intTimestamps)
	if Timestamp(intTimestamps[5]) > b.Timestamp {
		s.BadBlocks[b.ID()] = struct{}{}
		err = errors.New("timestamp invalid for being in the past")
		return
	}

	blockHash := b.ID()
	if bytes.Compare(parent.Target[:], blockHash[:]) < 0 {
		err = errors.New("block does not meet target")
		return
	}

	return
}

// Add a block to the state struct.
func (s *State) AcceptBlock(b *Block) (err error) {
	// Check the maps in the state to see if the block is already known.
	parentBlockNode, err := s.checkMaps(b)
	if err != nil {
		return
	}

	// Check that the header of the block is valid.
	err = s.validateHeader(parentBlockNode, b)
	if err != nil {
		return
	}

	/////////// Can be made into a function for adding a block to the tree.
	// Add the block to the block tree.
	newBlockNode := new(BlockNode)
	newBlockNode.Block = b
	parentBlockNode.Children = append(parentBlockNode.Children, newBlockNode)
	// newBlockNode.Verified = false // implicit value, stated explicity for prosperity.
	newBlockNode.Height = parentBlockNode.Height + 1
	copy(newBlockNode.RecentTimestamps[:], parentBlockNode.RecentTimestamps[1:])
	newBlockNode.RecentTimestamps[10] = b.Timestamp
	s.BlockMap[b.ID()] = newBlockNode

	///////////// Can be made into a function for calculating adjusted difficulty.
	var timePassed Timestamp
	var expectedTimePassed Timestamp
	var blockWindow BlockHeight
	if newBlockNode.Height < 5000 {
		// Calculate new target, using block 0 timestamp.
		timePassed = b.Timestamp - s.BlockRoot.Block.Timestamp
		expectedTimePassed = TargetSecondsPerBlock * Timestamp(newBlockNode.Height)
		blockWindow = newBlockNode.Height
	} else {
		// Calculate new target, using block Height-5000 timestamp.
		timePassed = b.Timestamp - s.BlockMap[s.ConsensusState.CurrentPath[newBlockNode.Height-5000]].Block.Timestamp
		expectedTimePassed = TargetSecondsPerBlock * 5000
		blockWindow = 5000
	}

	// Adjustment as a float = timePassed / expectedTimePassed / blockWindow.
	targetAdjustment := big.NewRat(int64(timePassed), int64(expectedTimePassed)*int64(blockWindow))

	// Enforce a maximum targetAdjustment
	if targetAdjustment.Cmp(MaxAdjustmentUp) == 1 {
		targetAdjustment = MaxAdjustmentUp
	} else if targetAdjustment.Cmp(MaxAdjustmentDown) == -1 {
		targetAdjustment = MaxAdjustmentDown
	}

	// Take the target adjustment and apply it to the target slice,
	// using rational numbers. Truncate the result.
	oldTarget := new(big.Int).SetBytes(parentBlockNode.Target[:])
	ratOldTarget := new(big.Rat).SetInt(oldTarget)
	ratNewTarget := ratOldTarget.Mul(targetAdjustment, ratOldTarget)
	intNewTarget := new(big.Int).Div(ratNewTarget.Num(), ratNewTarget.Denom())
	newTargetBytes := intNewTarget.Bytes()
	offset := len(newBlockNode.Target[:]) - len(newTargetBytes)
	copy(newBlockNode.Target[offset:], newTargetBytes)

	// Add the parent target to the depth of the block in the tree.
	blockWeight := new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).SetBytes(parentBlockNode.Target[:]))
	newBlockNode.Depth = BlockWeight(new(big.Rat).Add(parentBlockNode.Depth, blockWeight))

	///////////////// Can be made into a function for following a fork.
	// If the new node is .5% heavier than the other node, switch to the new fork.
	currentWeight := new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).SetBytes(s.BlockMap[s.ConsensusState.CurrentBlock].Target[:]))
	threshold := new(big.Rat).Mul(currentWeight, SurpassThreshold)
	requiredDepth := new(big.Rat).Add(s.BlockMap[s.ConsensusState.CurrentBlock].Depth, threshold)
	if (*big.Rat)(newBlockNode.Depth).Cmp(requiredDepth) == 1 {
		// Find the common parent between the new fork and the current
		// fork, keeping track of which path is taken through the
		// children of the parents so that we can re-trace as we
		// validate the blocks.
		currentNode := parentBlockNode
		value := s.ConsensusState.CurrentPath[currentNode.Height]
		var parentHistory []BlockID
		for value != currentNode.Block.ID() {
			parentHistory = append(parentHistory, currentNode.Block.ID())
			currentNode = s.BlockMap[currentNode.Block.ParentBlock]
			value = s.ConsensusState.CurrentPath[currentNode.Height]
		}

		// Remove blocks from the ConsensusState until we get to the
		// same parent that we are forking from.
		var rewoundBlocks []BlockID
		for s.ConsensusState.CurrentBlock != currentNode.Block.ID() {
			rewoundBlocks = append(rewoundBlocks, s.ConsensusState.CurrentBlock)
			s.RewindABlock()
		}

		// Validate each block in the parent history in order, updating
		// the state as we go.  If at some point a block doesn't
		// verify, you get to walk all the way backwards and forwards
		// again.
		validatedBlocks := 0
		for i := len(parentHistory) - 1; i >= 0; i-- {
			err = s.ValidateBlock(b)
			if err != nil {
				// Add the whole tree of blocks to BadBlocks,
				// deleting them from BlockMap

				// Rewind the validated blocks
				for i := 0; i < validatedBlocks; i++ {
					s.RewindABlock()
				}

				// Integrate the rewound blocks
				for i := len(rewoundBlocks) - 1; i >= 0; i-- {
					err = s.ValidateBlock(s.BlockMap[rewoundBlocks[i]].Block)
					if err != nil {
						panic(err)
					}
				}

				break
			}
			validatedBlocks += 1
		}

		// Do something to the transaction pool.
	} else {
		// Do something to the transaction pool.
	}

	return
}

// ValidateBlock will both verify the block AND update the consensus state.
// Calling integrate block is not needed.
func (s *State) ValidateBlock(b *Block) (err error) {
	// Check the hash on the merkle tree of transactions.

	var appliedTransactions []Transaction
	minerSubsidy := Currency(0)
	for _, txn := range b.Transactions {
		err = s.ValidateTxn(txn, s.BlockMap[b.ID()].Height)
		if err != nil {
			s.BadBlocks[b.ID()] = struct{}{}
			break
		}

		// Apply the transaction to the ConsensusState, adding it to the list of applied transactions.
		s.ApplyTransaction(txn)
		appliedTransactions = append(appliedTransactions, txn)

		minerSubsidy += txn.MinerFee
	}

	if err != nil {
		// Rewind transactions added to ConsensusState.
		for i := len(appliedTransactions) - 1; i >= 0; i-- {
			s.ReverseTransaction(appliedTransactions[i])
		}
		return
	}

	// Add outputs for all of the missed proofs in the open transactions.

	// Add coin inflation to the miner subsidy.

	// Add output contianing miner fees + block subsidy.
	bid := b.ID()
	minerSubsidyID := OutputID(HashBytes(append(bid[:], []byte("blockReward")...)))
	minerSubsidyOutput := Output{
		Value:     minerSubsidy,
		SpendHash: b.MinerAddress,
	}
	s.ConsensusState.UnspentOutputs[minerSubsidyID] = minerSubsidyOutput

	// s.BlockMap[b.ID()].Verified = true

	s.ConsensusState.CurrentBlock = b.ID()
	s.ConsensusState.CurrentPath[s.BlockMap[b.ID()].Height] = b.ID()

	return
}

// Add a function that integrates a block without verifying it.

/// Can probably split the validation of each piece into a different function,
//but perhaps not.
func (s *State) ValidateTxn(t Transaction, currentHeight BlockHeight) (err error) {
	inputSum := Currency(0)
	outputSum := t.MinerFee
	var inputSignaturesMap map[OutputID]InputSignatures
	for _, input := range t.Inputs {
		utxo, exists := s.ConsensusState.UnspentOutputs[input.OutputID]
		if !exists {
			err = errors.New("Transaction spends a nonexisting output")
			return
		}

		inputSum += utxo.Value

		// Check that the spend conditions match the hash listed in the output.

		// Check the timelock on the spend conditions is expired.
		if input.SpendConditions.TimeLock < currentHeight {
			err = errors.New("Output spent before timelock expiry.")
			return
		}

		// Create the condition for the input signatures and add it to the input signatures map.
		_, exists = inputSignaturesMap[input.OutputID]
		if exists {
			err = errors.New("Output spent twice in same transaction")
			return
		}
		var newInputSignatures InputSignatures
		newInputSignatures.RemainingSignatures = input.SpendConditions.NumSignatures
		newInputSignatures.PossibleKeys = input.SpendConditions.PublicKeys
		inputSignaturesMap[input.OutputID] = newInputSignatures
	}

	for _, output := range t.Outputs {
		outputSum += output.Value
	}

	for _, contract := range t.FileContracts {
		if contract.Start < currentHeight {
			err = errors.New("Contract starts in the future.")
			return
		}
		if contract.End <= contract.Start {
			err = errors.New("Contract duration must be at least one block.")
			return
		}
	}

	/*
		for _, proof := range t.StorageProofs {
			// Check that the proof passes.
			// Check that the proof has not already been submitted.
		}
	*/

	if inputSum != outputSum {
		err = errors.New("Inputs do not equal outputs for transaction.")
		return
	}

	for _, sig := range t.Signatures {
		// Check that each signature signs a unique pubkey where
		// RemainingSignatures > 0.
		if inputSignaturesMap[sig.InputID].RemainingSignatures == 0 {
			err = errors.New("Friviolous Signature detected.")
			return
		}
		_, exists := inputSignaturesMap[sig.InputID].UsedKeys[sig.PublicKeyIndex]
		if exists {
			err = errors.New("public key used twice while signing")
			return
		}

		// Check the timelock on the signature.
		if sig.TimeLock < currentHeight {
			err = errors.New("signature timelock has not expired")
			return
		}

		// Check that the actual signature is valid, following the covered fields struct.
	}

	return
}

func (s *State) ApplyTransaction(t Transaction) {
	// Remove all inputs from the unspent outputs list
	for _, input := range t.Inputs {
		s.ConsensusState.SpentOutputs[input.OutputID] = s.ConsensusState.UnspentOutputs[input.OutputID]
		delete(s.ConsensusState.UnspentOutputs, input.OutputID)
	}

	// Add all outputs to the unspent outputs list
	for i, output := range t.Outputs {
		newOutputID := OutputID(HashBytes(append((t.Inputs[0].OutputID)[:], EncUint64(uint64(i))...)))
		s.ConsensusState.UnspentOutputs[newOutputID] = output
	}

	// Add all outputs created by storage proofs.
	/*
		for _, sp := range t.StorageProofs {
			// Need to check that the contract fund has sufficient funds remaining.

			newOutputID := HashBytes(append(ContractID), []byte(n))
			output := Output {
				Value: s.ConsensusState.OpenContracts[sp.ContractID].ValidProofPayout,
				SpendHash: s.ConsensusState.OpenContracts[sp.ContractID].ValidProofAddress,
			}
			s.ConsensusState.UnspentOutputs[newOutputID] = output

			// need a counter or some way to determine what the index of
			// the window is.
		}
	*/
}

// Pulls just this transaction out of the ConsensusState.
func (s *State) ReverseTransaction(t Transaction) {
	// Remove all outputs created by storage proofs.

	// Remove all outputs created by outputs.
	for i := range t.Outputs {
		outputID := OutputID(HashBytes(append((t.Inputs[0].OutputID)[:], EncUint64(uint64(i))...)))
		delete(s.ConsensusState.UnspentOutputs, outputID)
	}

	// Add all outputs spent by inputs.
	for _, input := range t.Inputs {
		s.ConsensusState.UnspentOutputs[input.OutputID] = s.ConsensusState.SpentOutputs[input.OutputID]
		delete(s.ConsensusState.SpentOutputs, input.OutputID)
	}
}

// Pulls the most recent block out of the ConsensusState.
func (s *State) RewindABlock() {
	block := s.BlockMap[s.ConsensusState.CurrentBlock].Block
	for i := len(block.Transactions) - 1; i >= 0; i-- {
		s.ReverseTransaction(block.Transactions[i])
	}

	s.ConsensusState.CurrentBlock = block.ParentBlock
	delete(s.ConsensusState.CurrentPath, s.BlockMap[block.ID()].Height)
}
