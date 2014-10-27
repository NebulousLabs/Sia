package sia

import (
	"errors"
)

// Used to keep track of how many signatures an input has been signed by.
type InputSignatures struct {
	RemainingSignatures uint8
	PossibleKeys []PublicKey
	UsedKeys map[uint8]struct{}
}

// Add a block to the state struct.
func (s *State) AcceptBlock(b Block) (err error) {
	bid := b.ID() // Function is not implemented.

	_, exists := s.BadBlocks[bid]
	if exists {
		err = errors.New("Block is in bad list")
		return
	}

	if b.Version != 1 {
		s.BadBlocks[bid] = struct{}{}
		err = errors.New("Block is not version 1")
		return
	}

	// If timestamp is in the future, store in future blocks list.

	_, exists = s.BlockMap[bid]
	if exists {
		err = errors.New("Block exists in block map.")
		return
	}

	/*_, exists = s.OrphanBlocks[bid]
	if exists {
		err = errors.New("Block exists in orphan list")
		return
	} */

	prevNode, exists := s.BlockMap[b.Prevblock]
	if !exists {
		// OrphanBlocks[bid] = b
		err = errors.New("Block is an orphan")
		return
	}

	// Check the amount of work done by the block.

	// Add the block to the block tree.
	newBlockNode = new(BlockNode)
	newBlockNode.Block = b
	// newBlockNode.Verified = false // implicit value, stated explicity for prosperity.
	parentBlockNode = s.BlockMap[b.ParentBlock]
	newBlockNode.Height = parentBlockNode.Height + 1
	parentBlockNode.Children = append(parentBlockNode.Children, newBlockNode)

	// If block breaks forking threshold, integrate set of blocks.
}

// ValidateBlock will both verify the block AND update the consensus state.
// Calling integrate block is not needed.
func (s *State) ValidateBlock(b Block) (err error) {
	// Check the hash on the merkle tree of transactions.

	var appliedTransactions []Transaction
	minerSubsidy := 0
	for _, txn := range b.Transactions {
		err = s.ValidateTxn(txn, s.BlockMap[b.ID()].Height)
		if err != nil {
			s.BadBlocks[bid] = struct{}{}
			break
		}

		// Apply the transaction to the ConsensusState, adding it to the list of applied transactions.
		s.ApplyTransaction(txn)
		appliedTransactions = append(appliedTransactions, txn)

		minerSubsidy += txn.MinerFee
	}

	if err != nil {
		// Rewind transactions added to ConsensusState.
		return
	}

	// Add outputs for all of the missed proofs in the open transactions.

	// Add coin inflation to the miner subsidy.

	// Add output contianing miner fees + block subsidy.
	minerSubsidyID = append(b.ID(), []byte("minerSubsidy"))
	minerSubsidyOutput := Output {
		Value: minerSubsidy,
		SpendConditions: b.MinerAddress,
	}
	s.ConsensusState.UnspentOutputs[minerSubsidyID] = minerSubsidyOutput

	// s.BlockMap[b.ID()].Verified = true

	return
}

// Add a function that integrates a block without verifying it.

func (s *State) ValidateTxn(t Transaction, currentHeight uint32) (err error) {
	if t.Version != 1 {
		err = errors.New("Transaction version is not recognized.")
		return
	}

	inputSum := 0
	outputSum := t.MinerFee
	var inputSignaturesMap map[OutputID]InputSignatures
	for _, input := range Inputs {
		utxo, exists := s.ConsensusState[input.OutputID]
		if !exists {
			err = errors.New("Transaction spends a nonexisting output")
			return
		}

		inputSum += utxo.Value

		// Check that the spend conditions match the hash listed in the output.

		// Check the timelock on the spend conditions is expired.

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
		outputSum := contract.ContractFund
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
		if sig.Timelock <= currentHeight {
			err = errors.New("signature timelock has not expired")
			return
		}

		// Check that the actual signature is valid, following the covered fields struct.
	}
}

func (s *State) ApplyTransaction(t Transaction) {
	// Remove all inputs from the unspent outputs list
	for _, input := range t.Inputs {
		delete(s.ConsensusState.UnspentOutputs, input.OutputID)
	}

	// Add all outputs to the unspent outputs list
	for i, output := range t.Outputs {
		newOutputID := HashBytes(append(t.Inputs[0], []byte(i)))
		s.ConsensusState.UnspentOutputs[newOutputID] = output
	}

	// Add all outputs created by storage proofs.
	for _, sp := range t.StorageProofs {
		// Need to check that the contract fund has sufficient funds remaining.

		newOutputID := HashBytes(append(ContractID), []byte(n))
		output := Output {
			Value: s.ConsensusState.OpenContracts[sp.ContractID].ValidProofPayout,
			SpendHash: s.ConsensusState.OpenContracts[sp.ContractID].ValidProofAddress,
		}
		s.ConsensusState.UnspentOutputs[newOutputID] = output
	}
}
