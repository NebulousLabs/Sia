package consensus

import (
	"errors"
	"math/big"

	"github.com/NebulousLabs/Sia/hash"
)

// storageProofSegment takes a contractID and a windowIndex and calculates the
// index of the segment that should be proven on when doing a proof of storage.
func (s *State) storageProofSegment(contractID ContractID) (index uint64, err error) {
	contract, exists := s.openContracts[contractID]
	if !exists {
		err = errors.New("unrecognized contractID")
		return
	}

	// Get the id of the block used as the seed.
	triggerHeight := contract.Start - 1
	triggerBlock, exists := s.blockAtHeight(triggerHeight)
	if !exists {
		err = errors.New("no block found at contract trigger block height")
		return
	}
	triggerID := triggerBlock.ID()

	// Combine the contractID and triggerID, convert to an int, then take the
	// mod to get the segment.
	seed := hash.HashBytes(append(triggerID[:], contractID[:]...))
	numSegments := int64(hash.CalculateSegments(contract.FileSize))
	seedInt := new(big.Int).SetBytes(seed[:])
	index = seedInt.Mod(seedInt, big.NewInt(numSegments)).Uint64()
	return
}

// validContract returns err = nil if the contract is valid in the current
// context of the state, and returns an error if something about the contract
// is invalid.
func (s *State) validContract(fc FileContract) error {
	if fc.Start <= s.height() {
		return errors.New("contract must start in the future.")
	}
	if fc.End <= fc.Start {
		return errors.New("contract duration must be at least one block.")
	}
	return nil
}

// validProof returns err = nil if the storage proof provided is valid given
// the state context, otherwise returning an error to indicate what is invalid.
func (s *State) validProof(sp StorageProof) error {
	contract, exists := s.openContracts[sp.ContractID]
	if !exists {
		return errors.New("unrecognized contract id in storage proof")
	}

	// Check that the storage proof itself is valid.
	segmentIndex, err := s.storageProofSegment(sp.ContractID)
	if err != nil {
		return err
	}
	verified := hash.VerifyReaderProof(
		sp.Segment,
		sp.HashSet,
		hash.CalculateSegments(contract.FileSize),
		segmentIndex,
		contract.FileMerkleRoot,
	)
	if !verified {
		return errors.New("provided storage proof is invalid")
	}

	return nil
}

// addContract takes a FileContract and its corresponding ContractID and adds
// it to the state.
func (s *State) applyContract(contract FileContract, id ContractID) (cd ContractDiff) {
	s.openContracts[id] = contract
	cd = ContractDiff{
		New:      true,
		ID:       id,
		Contract: contract,
	}
	return
}

// applyStorageProof takes a storage proof and adds any outputs created by it
// to the consensus state.
func (s *State) applyStorageProof(sp StorageProof) (od OutputDiff, cd ContractDiff) {
	// Calculate the new output and its id.
	contract := s.openContracts[sp.ContractID]
	output := Output{
		Value:     contract.Payout,
		SpendHash: contract.ValidProofAddress,
	}
	outputID := sp.ContractID.StorageProofOutputID(true)

	// Update the state.
	s.unspentOutputs[outputID] = output
	delete(s.openContracts, sp.ContractID)

	od = OutputDiff{
		New:    true,
		ID:     outputID,
		Output: output,
	}
	cd = ContractDiff{
		New:      false,
		ID:       sp.ContractID,
		Contract: contract,
	}
	return
}

// applyMissedProof adds outputs to the State to manage a missed storage proof
// on a file contract.
func (s *State) applyMissedProof(contract FileContract, id ContractID) (od OutputDiff, cd ContractDiff) {
	// Create the output for the missed proof.
	output := Output{
		Value:     contract.Payout,
		SpendHash: contract.MissedProofAddress,
	}
	outputID := id.StorageProofOutputID(false)

	// Update the state.
	s.unspentOutputs[outputID] = output
	delete(s.openContracts, id)

	cd = ContractDiff{
		New:      false,
		ID:       id,
		Contract: contract,
	}
	od = OutputDiff{
		New:    true,
		ID:     outputID,
		Output: output,
	}
	return
}

func (s *State) applyContractMaintenance() (outputDiffs []OutputDiff, contractDiffs []ContractDiff) {
	// Iterate through all contracts and figure out which ones have expired.
	// Expiring a contract deletes it from the map we are iterating through, so
	// we need to store it and deleted once we're done iterating through the
	// map.
	var expiredContracts []ContractID
	for id, contract := range s.openContracts {
		if s.height() == contract.End {
			expiredContracts = append(expiredContracts, id)
		}
	}

	// Delete all of the contracts that terminated.
	for _, id := range expiredContracts {
		contract := s.openContracts[id]
		outputDiff, contractDiff := s.applyMissedProof(contract, id)
		outputDiffs = append(outputDiffs, outputDiff)
		contractDiffs = append(contractDiffs, contractDiff)
	}

	return
}

// StorageProofSegmentIndex takes a contractID and a windowIndex and calculates
// the index of the segment that should be proven on when doing a proof of
// storage.
func (s *State) StorageProofSegment(id ContractID) (index uint64, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.storageProofSegment(id)
}

func (s *State) ValidContract(fc FileContract) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.validContract(fc)
}
