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
//
// Currently txns are not harvested from bad blocks. Good txns should be
// harvested from bad blocks.
func (s *State) IncorporateBlock(b Block) (err error) {
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
	newBlockNode.Verified = false // implicit value, stated explicity for prosperity.
	parentBlockNode = s.BlockMap[b.ParentBlock]
	parentBlockNode.Children = append(parentBlockNode.Children, newBlockNode)

	// If block breaks forking threshold, validate set of blocks.

}

// ValidateBlock will both verify the block AND update the consensus state.
// Updating ConsensusState is not necesary.
func (s *State) ValidateBlock(b Block) (err error) {
	// Check the hash on the merkle tree of transactions.

	for _, txn := range b.Transactions {
		err = s.ValidateTxn(txn)
		if err != nil {
			s.BadBlocks[bid] = struct{}{}
			break
		}

		// Apply the transaction to the ConsensusState, adding it to the list of applied transactions.
	}

	if err != nil {
		// Rewind transactions added to ConsensusState.
		return
	}

	// Add outputs for all of the missed proofs in the open transactions.

	s.BlockMap[b.ID()].Verified = true
	return
}

func (s *State) ValidateTxn(t Transaction) (err error) {
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

		// Check that start is in the future.
		// Check that end is after start.
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
		// Check that each signature adds value to the input. (signs a unique public key, isn't frivilous)

		// Check the timelock on the signature.

		// Check that the actual signature is valid, following the covered fields struct.
	}
}
