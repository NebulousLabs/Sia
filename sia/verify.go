package sia

import (
	"bytes"
	"errors"
	"math/big"
	"sort"
	"time"

	"github.com/NebulousLabs/Andromeda/encoding"
)

// Each input has a list of public keys and a required number of signatures.
// This struct keeps track of which public keys have been used and how many
// more signatures are needed.
type InputSignatures struct {
	RemainingSignatures uint64
	PossibleKeys        []PublicKey
	UsedKeys            map[uint64]struct{}
}

// State.validTransaction returns err = nil if the transaction is valid, otherwise
// returns an error explaining what wasn't valid.
func (s *State) validTransaction(t *Transaction) (err error) {
	// Iterate through each input, summing the value, checking for
	// correctness, and creating an InputSignatures object.
	inputSum := Currency(0)
	inputSignaturesMap := make(map[OutputID]InputSignatures)
	for _, input := range t.Inputs {
		// Check the input spends an existing and valid output.
		utxo, exists := s.UnspentOutputs[input.OutputID]
		if !exists {
			err = errors.New("transaction spends a nonexisting output")
			return
		}

		// Check that the spend conditions match the hash listed in the output.
		if input.SpendConditions.CoinAddress() != s.UnspentOutputs[input.OutputID].SpendHash {
			err = errors.New("spend conditions do not match hash")
			return
		}

		// Check the timelock on the spend conditions is expired.
		if input.SpendConditions.TimeLock > s.Height() {
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

		// Add the input to the coin sum.
		inputSum += utxo.Value
	}

	// Tally up the miner fees and output values.
	outputSum := Currency(0)
	for _, minerFee := range t.MinerFees {
		outputSum += minerFee
	}
	for _, output := range t.Outputs {
		outputSum += output.Value
	}

	// Verify the contracts and tally up the expenditures.
	for _, contract := range t.FileContracts {
		if contract.ContractFund < 0 {
			err = errors.New("contract must be funded.")
			return
		}
		if contract.Start < s.Height() {
			err = errors.New("contract must start in the future.")
			return
		}
		if contract.End <= contract.Start {
			err = errors.New("contract duration must be at least one block.")
			return
		}

		outputSum += contract.ContractFund
	}

	for _, proof := range t.StorageProofs {
		// Check that the proof has not already been submitted.
		if s.OpenContracts[proof.ContractID].WindowSatisfied {
			err = errors.New("storage proof has already been completed for this contract")
			return
		}

		// Check that the proof passes.
	}

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
		if sig.TimeLock > s.Height() {
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

// State.addTransactionToPool() adds a transaction to the transaction pool and
// transaction list without verifying it.
func (s *State) addTransactionToPool(t *Transaction) {
	for _, input := range t.Inputs {
		s.TransactionPool[input.OutputID] = t
	}
	s.TransactionList[t.Inputs[0].OutputID] = t
}

// Takes a transaction out of the transaction pool & transaction list.
func (s *State) removeTransactionFromPool(t *Transaction) {
	for _, input := range t.Inputs {
		delete(s.TransactionPool, input.OutputID)
	}
	delete(s.TransactionList, t.Inputs[0].OutputID)
}

// State.AcceptTransaction() checks for a conflict of the transaction with the
// transaction pool, then checks that the transaction is valid given the
// current state, then adds the transaction to the transaction pool.
// AcceptTransaction() is thread safe, and can be called concurrently.
func (s *State) AcceptTransaction(t Transaction) (err error) {
	s.Lock()
	defer s.Unlock()

	// Check that the transaction is not in conflict with the transaction
	// pool.
	for _, input := range t.Inputs {
		_, exists := s.TransactionPool[input.OutputID]
		if exists {
			err = errors.New("conflicting transaction exists in transaction pool")
			return
		}
	}

	// Check that the transaction is potentially valid.
	err = s.validTransaction(&t)
	if err != nil {
		return
	}

	// Add the transaction to the pool.
	s.addTransactionToPool(&t)

	// forward transaction to peers
	s.Server.Broadcast(SendVal('T', t))

	return
}

// State.checkMaps() looks through the maps known to the state and sees if the
// block id has been cached anywhere.
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

// Block.expectedTransactionMerkleRoot() returns the expected transaction
// merkle root of the block.
func (b *Block) expectedTransactionMerkleRoot() Hash {
	var transactionHashes []Hash
	for _, transaction := range b.Transactions {
		transactionHashes = append(transactionHashes, HashBytes(encoding.Marshal(transaction)))
	}
	return MerkleRoot(transactionHashes)
}

// Block.checkTarget() returns true if the block id is lower than the target.
func (b *Block) checkTarget(target Target) bool {
	blockHash := b.ID()
	return bytes.Compare(target[:], blockHash[:]) >= 0
}

// State.validateHaeader() returns `err = nil` if the header information in the
// block (everything except the transactions) is valid, and returns an error
// explaining why validation failed if the header is invalid.
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

	// Check that the transaction merkle root matches the transactions
	// included into the block.
	if b.MerkleRoot != b.expectedTransactionMerkleRoot() {
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

// State.childTarget() calculates the proper target of a child node given the
// parent node, and copies the target into the child node.
func (s *State) childTarget(parentNode *BlockNode, newNode *BlockNode) (target Target) {
	var timePassed, expectedTimePassed Timestamp
	if newNode.Height < TargetWindow {
		timePassed = newNode.Block.Timestamp - s.BlockRoot.Block.Timestamp
		expectedTimePassed = BlockFrequency * Timestamp(newNode.Height)
	} else {
		// WARNING: this code assumes that the block at height
		// newNode.Height-TargetWindow is the same for both the new
		// node and the currenct fork. In general, this is a safe
		// assumption, because there should never be a reorg that's
		// 5000 blocks long.
		adjustmentBlock := s.blockAtHeight(newNode.Height - TargetWindow)
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

// State.childDepth() returns the cumulative weight of all the blocks leading
// up to and including the child block.
func (s *State) childDepth(parentNode *BlockNode) (depth Target) {
	blockWeight := new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).SetBytes(parentNode.Target[:])) // WRITE A FUNCTION TO GO FROM TARGET TO WEIGHT
	ratParentDepth := new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).SetBytes(parentNode.Depth[:]))
	ratChildDepth := new(big.Rat).Add(ratParentDepth, blockWeight)
	intChildDepth := new(big.Int).Div(ratChildDepth.Denom(), ratChildDepth.Num())
	bytesChildDepth := intChildDepth.Bytes()
	offset := len(depth[:]) - len(bytesChildDepth[:])
	copy(depth[offset:], bytesChildDepth[:])
	return
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
	s.BlockMap[b.ID()] = newNode
	parentNode.Children = append(parentNode.Children, newNode)

	return
}

// State.heavierFork() returns ture if the input node is 5% heavier than the
// current node of the ConesnsusState.
func (s *State) heavierFork(newNode *BlockNode) bool {
	threshold := new(big.Rat).Mul(s.currentBlockWeight(), SurpassThreshold)
	sdepth := s.Depth()
	currentDepth := new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).SetBytes(sdepth[:]))
	requiredDepth := new(big.Rat).Add(currentDepth, threshold)
	newNodeDepth := new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).SetBytes(newNode.Depth[:]))
	return newNodeDepth.Cmp(requiredDepth) == 1
}

// State.reverseTransaction removes a given transaction from the
// ConsensusState, making it as though the transaction had never happened.
func (s *State) reverseTransaction(t Transaction) {
	// Remove all outputs.
	for i := range t.Outputs {
		delete(s.UnspentOutputs, t.OutputID(i))
	}

	// Add all outputs spent by inputs.
	for _, input := range t.Inputs {
		s.UnspentOutputs[input.OutputID] = s.SpentOutputs[input.OutputID]
		delete(s.SpentOutputs, input.OutputID)
	}

	// Delete all outputs created by storage proofs.
	for _, sp := range t.StorageProofs {
		openContract := s.OpenContracts[sp.ContractID]
		outputID, err := openContract.FileContract.StorageProofOutputID(openContract.ContractID, s.Height(), true)
		if err != nil {
			panic(err)
		}
		delete(s.UnspentOutputs, outputID)
	}

	// Delete all the open contracts created by new contracts.
	for i := range t.FileContracts {
		contractID := t.FileContractID(i)
		delete(s.OpenContracts, contractID)
	}
}

// State.rewindABlock() removes the most recent block from the ConsensusState,
// making the ConsensusState as though the block had never been integrated.
func (s *State) rewindABlock() {
	// Repen all contracts that terminated, and remove the corresponding output.
	for _, openContract := range s.currentBlockNode().ContractTerminations {
		s.OpenContracts[openContract.ContractID] = openContract
		contractStatus := openContract.Failures == openContract.FileContract.Tolerance
		delete(s.UnspentOutputs, openContract.FileContract.ContractTerminationOutputID(openContract.ContractID, contractStatus))
	}

	// Reverse all outputs created by missed storage proofs.
	for _, missedProof := range s.currentBlockNode().MissedStorageProofs {
		s.OpenContracts[missedProof.ContractID].FundsRemaining += s.UnspentOutputs[missedProof.OutputID].Value
		s.OpenContracts[missedProof.ContractID].Failures -= 1
		delete(s.UnspentOutputs, missedProof.OutputID)
	}

	// Reverse each transaction in the block, in reverse order from how
	// they appear in the block.
	for i := len(s.currentBlock().Transactions) - 1; i >= 0; i-- {
		s.reverseTransaction(s.currentBlock().Transactions[i])
		s.addTransactionToPool(&s.currentBlock().Transactions[i])
	}

	// Update the CurrentBlock and CurrentPath variables of the longest fork.
	s.CurrentBlock = s.currentBlock().ParentBlock
	delete(s.CurrentPath, s.Height())
}

// State.applyTransaction() takes a transaction and adds it to the
// ConsensusState, updating the list of contracts, outputs, etc.
func (s *State) applyTransaction(t Transaction) {
	// Remove all inputs from the unspent outputs list.
	for _, input := range t.Inputs {
		s.SpentOutputs[input.OutputID] = s.UnspentOutputs[input.OutputID]
		delete(s.UnspentOutputs, input.OutputID)
	}

	// REMOVE ALL CONFLICTING TRANSACTIONS FROM THE TRANSACTION POOL.

	// Add all outputs to the unspent outputs list
	for i, output := range t.Outputs {
		s.UnspentOutputs[t.OutputID(i)] = output
	}

	// Add all new contracts to the OpenContracts list.
	for i, contract := range t.FileContracts {
		contractID := t.FileContractID(i)
		openContract := OpenContract{
			FileContract:    contract,
			ContractID:      contractID,
			FundsRemaining:  contract.ContractFund,
			Failures:        0,
			WindowSatisfied: true, // The first window is free, because the start is in the future by mandate.
		}
		s.OpenContracts[contractID] = &openContract
	}

	// Add all outputs created by storage proofs.
	for _, sp := range t.StorageProofs {
		// Check for contract termination.
		openContract := s.OpenContracts[sp.ContractID]
		payout := openContract.FileContract.ValidProofPayout
		if openContract.FundsRemaining < openContract.FileContract.ValidProofPayout {
			payout = openContract.FundsRemaining
		}

		output := Output{
			Value:     payout,
			SpendHash: openContract.FileContract.ValidProofAddress,
		}
		outputID, err := openContract.FileContract.StorageProofOutputID(openContract.ContractID, s.Height(), true)
		if err != nil {
			panic(err)
		}
		s.UnspentOutputs[outputID] = output

		// Mark the proof as complete for this window.
		s.OpenContracts[sp.ContractID].WindowSatisfied = true
		s.OpenContracts[sp.ContractID].FundsRemaining -= payout
	}

	// Check the arbitrary data of the transaction to fill out the host database.
	if len(t.ArbitraryData) > 8 {
		dataIndicator := encoding.DecUint64(t.ArbitraryData[0:8])
		if dataIndicator == 1 {
			var ha HostAnnouncement
			encoding.Unmarshal(t.ArbitraryData[1:], ha)

			// Verify that the spend condiitons match.
			if ha.SpendConditions.CoinAddress() != t.Outputs[ha.FreezeIndex].SpendHash {
				return
			}

			// Add the host to the host database.
			host := Host{
				IPAddress:   string(ha.IPAddress),
				MinSize:     ha.MinFilesize,
				MaxSize:     ha.MaxFilesize,
				Duration:    ha.MaxDuration,
				Frequency:   ha.MaxChallengeFrequency,
				Tolerance:   ha.MinTolerance,
				Price:       ha.Price,
				Burn:        ha.Burn,
				Freeze:      Currency(ha.SpendConditions.TimeLock-s.Height()) * t.Outputs[ha.FreezeIndex].Value,
				CoinAddress: ha.CoinAddress,
			}
			if host.Freeze <= 0 {
				return
			}

			// Add the weight of the host to the total weight of the hosts in
			// the host database.
			s.HostList = append(s.HostList, host)
			s.TotalWeight += host.Weight()
		}
	}
}

// s.integrateBlock() will verify the block and then integrate it into the
// consensus state.
func (s *State) integrateBlock(b *Block) (err error) {
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
		// Rewind transactions added to
		for i := len(appliedTransactions) - 1; i >= 0; i-- {
			s.reverseTransaction(appliedTransactions[i])
		}
		return
	}

	// Perform maintanence on all open contracts.
	//
	// This could be split into its own function.
	var contractsToDelete []ContractID
	for _, openContract := range s.OpenContracts {
		// Check for the window switching over.
		if (s.Height()-openContract.FileContract.Start)%openContract.FileContract.ChallengeFrequency == 0 && s.Height() > openContract.FileContract.Start {
			// Check for a missed proof.
			if openContract.WindowSatisfied == false {
				payout := openContract.FileContract.MissedProofPayout
				if openContract.FundsRemaining < openContract.FileContract.MissedProofPayout {
					payout = openContract.FundsRemaining
				}

				newOutputID, err := openContract.FileContract.StorageProofOutputID(openContract.ContractID, s.Height(), false)
				if err != nil {
					panic(err)
				}
				output := Output{
					Value:     payout,
					SpendHash: openContract.FileContract.MissedProofAddress,
				}
				s.UnspentOutputs[newOutputID] = output
				msp := MissedStorageProof{
					OutputID:   newOutputID,
					ContractID: openContract.ContractID,
				}
				s.currentBlockNode().MissedStorageProofs = append(s.currentBlockNode().MissedStorageProofs, msp)

				// Update the FundsRemaining
				openContract.FundsRemaining -= payout

				// Update the failures count.
				openContract.Failures += 1
			}
			openContract.WindowSatisfied = false
		}

		// Check for a terminated contract.
		if openContract.FundsRemaining == 0 || openContract.FileContract.End == s.Height() || openContract.FileContract.Tolerance == openContract.Failures {
			if openContract.FundsRemaining != 0 {
				// Create a new output that terminates the contract.
				contractStatus := openContract.Failures == openContract.FileContract.Tolerance // MAKE A FUNCTION TO GET THIS VALUE
				outputID := openContract.FileContract.ContractTerminationOutputID(openContract.ContractID, contractStatus)
				output := Output{
					Value: openContract.FundsRemaining,
				}
				if openContract.FileContract.Tolerance == openContract.Failures {
					output.SpendHash = openContract.FileContract.MissedProofAddress
				} else {
					output.SpendHash = openContract.FileContract.ValidProofAddress
				}
				s.UnspentOutputs[outputID] = output
			}

			// Add the contract to contract terminations.
			s.currentBlockNode().ContractTerminations = append(s.currentBlockNode().ContractTerminations, openContract)

			// Mark contract for deletion (can't delete from a map while
			// iterating through it - results in undefined behavior of the
			// iterator.
			contractsToDelete = append(contractsToDelete, openContract.ContractID)
		}
	}
	// Delete all of the contracts that terminated.
	for _, contractID := range contractsToDelete {
		delete(s.OpenContracts, contractID)
	}

	// Add coin inflation to the miner subsidy.
	minerSubsidy += 1000

	// Add output contianing miner fees + block subsidy.
	minerSubsidyOutput := Output{
		Value:     minerSubsidy,
		SpendHash: b.MinerAddress,
	}
	s.UnspentOutputs[b.SubsidyID()] = minerSubsidyOutput

	// Update the current block and current path variables of the longest fork.
	s.CurrentBlock = b.ID()
	s.CurrentPath[s.BlockMap[b.ID()].Height] = b.ID()

	return
}

// State.invalidateNode() is a recursive function that deletes all of the
// children of a block and puts them on the bad blocks list.
func (s *State) invalidateNode(node *BlockNode) {
	for i := range node.Children {
		s.invalidateNode(node.Children[i])
	}

	delete(s.BlockMap, node.Block.ID())
	s.BadBlocks[node.Block.ID()] = struct{}{}
}

// State.forkBlockchain() will go from the current block over to a block on a
// different fork, rewinding and integrating blocks as needed. forkBlockchain()
// will return an error if any of the blocks in the new fork are invalid.
func (s *State) forkBlockchain(newNode *BlockNode) (err error) {
	// Find the common parent between the new fork and the current
	// fork, keeping track of which path is taken through the
	// children of the parents so that we can re-trace as we
	// validate the blocks.
	currentNode := newNode
	value := s.CurrentPath[currentNode.Height]
	var parentHistory []BlockID
	for value != currentNode.Block.ID() {
		parentHistory = append(parentHistory, currentNode.Block.ID())
		currentNode = s.BlockMap[currentNode.Block.ParentBlock]
		value = s.CurrentPath[currentNode.Height]
	}

	// Remove blocks from the ConsensusState until we get to the
	// same parent that we are forking from.
	var rewoundBlocks []BlockID
	for s.CurrentBlock != currentNode.Block.ID() {
		rewoundBlocks = append(rewoundBlocks, s.CurrentBlock)
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
			s.invalidateNode(s.BlockMap[parentHistory[i]])

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

	return
}

// State.AcceptBlock() is a thread-safe function that will add blocks to the
// state, forking the blockchain if they are on a fork that is heavier than the
// current fork. AcceptBlock() can be called concurrently.
func (s *State) AcceptBlock(b Block) (err error) {
	s.Lock()
	defer s.Unlock()

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
		err = s.forkBlockchain(newBlockNode)
		if err != nil {
			return
		}
	}

	// forward block to peers
	s.Server.Broadcast(SendVal('B', b))

	return
}
