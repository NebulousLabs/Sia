package sia

import (
	"errors"
)

// Currently a stateless verification. State is needed to build a tree though.
func BlockVerify(b Block) {

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

	// Check the hash on the merkle tree of transactions.

	for _, txn := range b.Transactions {
		// Validate each transaction.
		err := s.ValidateTxn(txn)
		if err != nil {
			s.BadBlocks[bid] = struct{}{}
			return
		}
	}
}

// If you are validating the block, then the consensus state needs to be
// pointing at the current branch in the fork tree.
func (s *State) ValidateBlock(b Block) (err error) {
	// Move stuff from incorporate to here conditionally.
}

func (s *State) ValidateTxn(t Transaction) (err error) {
	if t.Version != 1 {
		err = errors.New("Transaction version is not recognized.")
		return
	}

	inputSum := 0
	outputSum := t.MinerFee
	for _, input := range Inputs {
		utxo, exists := s.ConsensusState[input.OutputID]
		if !exists {
			err = errors.New("Transaction spends a nonexisting output")
			return
		}

		inputSum += utxo.Value

		// Check that the spend conditions match the hash listed in the output.

		// Check the timelock on the spend conditions is expired.

		// Add the signature situation to some struct =/
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
	}

	for _, sig := range t.Signatures {
		// Check that each signature adds value to the input. (signs a unique public key, isn't frivilous)

		// Check the timelock on the signature.

		// Check that the actual signature is valid, following the covered fields struct.
	}
}
