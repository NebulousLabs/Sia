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

func (s *State) addTransactionToPool(t *Transaction) {
	for _, input := range t.Inputs {
		s.ConsensusState.TransactionPool[input.OutputID] = t
	}
	s.ConsensusState.TransactionList[t.Inputs[0].OutputID] = t
}

func (s *State) removeTransactionFromPool(t *Transaction) {
	for _, input := range t.Inputs {
		delete(s.ConsensusState.TransactionPool, input.OutputID)
	}
	delete(s.ConsensusState.TransactionList, t.Inputs[0].OutputID)
}

// Add a transaction to the state struct.
func (s *State) AcceptTransaction(t *Transaction) (err error) {
	s.Lock()
	defer s.Unlock()

	// Check that the transaction is not in conflict with the transaction
	// pool.
	for _, input := range t.Inputs {
		_, exists := s.ConsensusState.TransactionPool[input.OutputID]
		if exists {
			err = errors.New("conflicting transaction exists in transaction pool")
		}
	}

	// Check that the transaction is potentially valid.
	err = s.validTransaction(t)
	if err != nil {
		return
	}

	// Add the transaction to the pool.
	s.addTransactionToPool(t)

	return
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

// Return the expected merkle root of the block.
func (b *Block) expectedMerkleRoot() Hash {
	var transactionHashes []Hash
	for _, transaction := range b.Transactions {
		transactionHashes = append(transactionHashes, HashBytes(Marshal(transaction)))
	}
	return MerkleRoot(transactionHashes)
}

// Returns true if the block id fultills the target hash.
func (b *Block) checkTarget(target Target) bool {
	blockHash := b.ID()
	return bytes.Compare(target[:], blockHash[:]) >= 0
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

	// Check that the MerkleRoot matches the transactions.
	if b.MerkleRoot != b.expectedMerkleRoot() {
		s.BadBlocks[b.ID()] = struct{}{}
		err = errors.New("merkle root does not match transactions sent.")
		return
	}

	// Check the id meets the target.
	if !b.checkTarget(parent.Target) {
		err = errors.New("block does not meet target")
		return
	}

	return
}

// Calculates the target of a child depending on the timestamp of the child
// block.
func (s *State) childTarget(parentNode *BlockNode, newNode *BlockNode) (target Target) {
	var timePassed, expectedTimePassed Timestamp
	blockWindow := BlockHeight(5000)
	if newNode.Height < 5000 {
		timePassed = newNode.Block.Timestamp - s.BlockRoot.Block.Timestamp
		expectedTimePassed = TargetSecondsPerBlock * Timestamp(newNode.Height)
		blockWindow = newNode.Height
	} else {
		adjustmentBlock := s.BlockMap[s.ConsensusState.CurrentPath[newNode.Height-5000]].Block
		timePassed = newNode.Block.Timestamp - adjustmentBlock.Timestamp
		expectedTimePassed = TargetSecondsPerBlock * 5000
	}

	// Adjustment = timePassed / expectedTimePassed / blockWindow.
	targetAdjustment := big.NewRat(int64(timePassed), int64(expectedTimePassed)*int64(blockWindow))

	// Enforce a maximum targetAdjustment
	if targetAdjustment.Cmp(MaxAdjustmentUp) == 1 {
		targetAdjustment = MaxAdjustmentUp
	} else if targetAdjustment.Cmp(MaxAdjustmentDown) == -1 {
		targetAdjustment = MaxAdjustmentDown
	}

	// Take the target adjustment and apply it to the target slice,
	// using rational numbers. Truncate the result.
	oldTarget := new(big.Int).SetBytes(parentNode.Target[:])
	ratOldTarget := new(big.Rat).SetInt(oldTarget)
	ratNewTarget := ratOldTarget.Mul(targetAdjustment, ratOldTarget)
	intNewTarget := new(big.Int).Div(ratNewTarget.Num(), ratNewTarget.Denom())
	newTargetBytes := intNewTarget.Bytes()
	offset := len(target[:]) - len(newTargetBytes)
	copy(target[offset:], newTargetBytes)
	return
}

// Calculates the depth of a child given the parent node - note that the depth
// of the child is independant of the actual child block.
func (s *State) childDepth(parentNode *BlockNode) BlockWeight {
	blockWeight := new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).SetBytes(parentNode.Target[:]))
	return BlockWeight(new(big.Rat).Add(parentNode.Depth, blockWeight))
}

// Takes a block and a parent node, and adds the block as a child to the parent
// node.
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
	s.BlockMap[b.ID()] = newNode
	parentNode.Children = append(parentNode.Children, newNode)

	return
}

// Returns true if the input node is 5% heavier than the current node.
func (s *State) heavierFork(newNode *BlockNode) bool {
	currentWeight := new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).SetBytes(s.BlockMap[s.ConsensusState.CurrentBlock].Target[:]))
	threshold := new(big.Rat).Mul(currentWeight, SurpassThreshold)
	requiredDepth := new(big.Rat).Add(s.BlockMap[s.ConsensusState.CurrentBlock].Depth, threshold)
	return (*big.Rat)(newNode.Depth).Cmp(requiredDepth) == 1
}

// Pulls just this transaction out of the ConsensusState.
func (s *State) reverseTransaction(t Transaction) {
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
func (s *State) rewindABlock() {
	block := s.BlockMap[s.ConsensusState.CurrentBlock].Block
	for i := len(block.Transactions) - 1; i >= 0; i-- {
		s.reverseTransaction(block.Transactions[i])
		s.addTransactionToPool(&block.Transactions[i])
	}

	s.ConsensusState.CurrentBlock = block.ParentBlock
	delete(s.ConsensusState.CurrentPath, s.BlockMap[block.ID()].Height)
}

// ValidTransaction returns err = nil if the transaction is valid, otherwise
// returns an error explaining what wasn't valid.
func (s *State) validTransaction(t *Transaction) (err error) {
	currentHeight := s.BlockMap[s.ConsensusState.CurrentBlock].Height
	inputSum := Currency(0)
	var inputSignaturesMap map[OutputID]InputSignatures
	for _, input := range t.Inputs {
		utxo, exists := s.ConsensusState.UnspentOutputs[input.OutputID]
		if !exists {
			err = errors.New("transaction spends a nonexisting output")
			return
		}

		inputSum += utxo.Value

		// Check that the spend conditions match the hash listed in the output.
		if input.SpendConditions.Address() != s.ConsensusState.UnspentOutputs[input.OutputID].SpendHash {
			err = errors.New("spend conditions do not match hash")
			return
		}

		// Check the timelock on the spend conditions is expired.
		if input.SpendConditions.TimeLock < currentHeight {
			err = errors.New("output spent before timelock expiry.")
			return
		}

		// Create the condition for the input signatures and add it to the input signatures map.
		_, exists = inputSignaturesMap[input.OutputID]
		if exists {
			err = errors.New("output spent twice in same transaction")
			return
		}
		var newInputSignatures InputSignatures
		newInputSignatures.RemainingSignatures = input.SpendConditions.NumSignatures
		newInputSignatures.PossibleKeys = input.SpendConditions.PublicKeys
		inputSignaturesMap[input.OutputID] = newInputSignatures
	}

	outputSum := Currency(0)
	for _, minerFee := range t.MinerFees {
		outputSum += minerFee
	}

	for _, output := range t.Outputs {
		outputSum += output.Value
	}

	/*
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

		for _, proof := range t.StorageProofs {
			// Check that the proof passes.
			// Check that the proof has not already been submitted.
		}
	*/

	if inputSum != outputSum {
		err = errors.New("inputs do not equal outputs for transaction.")
		return
	}

	for i, sig := range t.Signatures {
		// Check that each signature signs a unique pubkey where
		// RemainingSignatures > 0.
		if inputSignaturesMap[sig.InputID].RemainingSignatures == 0 {
			err = errors.New("friviolous signature detected.")
			return
		}
		_, exists := inputSignaturesMap[sig.InputID].UsedKeys[sig.PublicKeyIndex]
		if exists {
			err = errors.New("public key used twice while signing")
			return
		}

		// Check the timelock on the signature.
		if sig.TimeLock > currentHeight {
			err = errors.New("signature timelock has not expired")
			return
		}

		// Check that the signature matches the public key.
		sigHash := t.SigHash(i)
		if !VerifyBytes(sigHash[:], inputSignaturesMap[sig.InputID].PossibleKeys[sig.PublicKeyIndex], sig.Signature) {
			err = errors.New("invalid signature in transaction")
			return
		}
	}

	return
}

// Takes a transaction and applies it to the ConsensusState. Should only be
// called in the context of applying a whole block.
func (s *State) applyTransaction(t Transaction) {
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

// integrateBlock will both verify the block AND update the consensus state.
// Calling integrate block is not needed.
func (s *State) integrateBlock(b *Block) (err error) {
	// Check the hash on the merkle tree of transactions.

	var appliedTransactions []Transaction
	minerSubsidy := Currency(0)
	for _, txn := range b.Transactions {
		err = s.validTransaction(&txn)
		if err != nil {
			s.BadBlocks[b.ID()] = struct{}{}
			break
		}

		// Apply the transaction to the ConsensusState, adding it to the list of applied transactions.
		s.applyTransaction(txn)
		appliedTransactions = append(appliedTransactions, txn)

		// Remove the inputs from the transaction pool.
		s.removeTransactionFromPool(&txn)

		// Add the miner fees to the miner subsidy.
		for _, fee := range txn.MinerFees {
			minerSubsidy += fee
		}
	}

	if err != nil {
		// Rewind transactions added to ConsensusState.
		for i := len(appliedTransactions) - 1; i >= 0; i-- {
			s.reverseTransaction(appliedTransactions[i])
		}
		return
	}

	// Add coin inflation to the miner subsidy.
	minerSubsidy += Currency(1000)

	// Add output contianing miner fees + block subsidy.
	bid := b.ID()
	minerSubsidyID := OutputID(HashBytes(append(bid[:], []byte("blockReward")...)))
	minerSubsidyOutput := Output{
		Value:     minerSubsidy,
		SpendHash: b.MinerAddress,
	}
	s.ConsensusState.UnspentOutputs[minerSubsidyID] = minerSubsidyOutput

	s.ConsensusState.CurrentBlock = b.ID()
	s.ConsensusState.CurrentPath[s.BlockMap[b.ID()].Height] = b.ID()

	return
}

func (s *State) forkBlockchain(parentNode *BlockNode) (err error) {
	// Find the common parent between the new fork and the current
	// fork, keeping track of which path is taken through the
	// children of the parents so that we can re-trace as we
	// validate the blocks.
	currentNode := parentNode
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
		s.rewindABlock()
	}

	// Validate each block in the parent history in order, updating
	// the state as we go.  If at some point a block doesn't
	// verify, you get to walk all the way backwards and forwards
	// again.
	validatedBlocks := 0
	for i := len(parentHistory) - 1; i >= 0; i-- {
		err = s.integrateBlock(s.BlockMap[parentHistory[i]].Block)
		if err != nil {
			// Add the whole tree of blocks to BadBlocks,
			// deleting them from BlockMap

			// Rewind the validated blocks
			for i := 0; i < validatedBlocks; i++ {
				s.rewindABlock()
			}

			// Integrate the rewound blocks
			for i := len(rewoundBlocks) - 1; i >= 0; i-- {
				err = s.integrateBlock(s.BlockMap[rewoundBlocks[i]].Block)
				if err != nil {
					panic("Once-validated blocks are no longer validating - state logic has mistakes.")
				}
			}

			break
		}
		validatedBlocks += 1
	}

	if err != nil {
		// Do something to the transaction pool.
	} else {
		// Maybe still do something to the transaction pool.
	}

	return
}

// Add a block to the state struct.
func (s *State) AcceptBlock(b *Block) (err error) {
	s.Lock()
	defer s.Unlock()

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

	newBlockNode := s.addBlockToTree(parentBlockNode, b)

	// If the new node is 5% heavier than the current node, switch to the new fork.
	if s.heavierFork(newBlockNode) {
		err = s.forkBlockchain(parentBlockNode)
		if err != nil {
			return
		}
	}

	return
}
