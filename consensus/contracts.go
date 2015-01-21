package consensus

import (
	"errors"
	"math/big"

	"github.com/NebulousLabs/Sia/hash"
)

// storageProofSegmentIndex takes a contractID and a windowIndex and calculates
// the index of the segment that should be proven on when doing a proof of
// storage.
func (s *State) storageProofSegmentIndex(contractID ContractID) (index uint64, err error) {
	contract, exists := s.openContracts[contractID]
	if !exists {
		err = errors.New("unrecognized contractID")
		return
	}

	// Get random number seed used to pick the index.
	triggerBlockHeight := contract.Start - 1
	triggerBlock, err := s.blockAtHeight(triggerBlockHeight)
	if err != nil {
		return
	}
	triggerBlockID := triggerBlock.ID()
	seed := hash.HashBytes(append(triggerBlockID[:], contractID[:]...))

	numSegments := int64(hash.CalculateSegments(contract.FileSize))
	seedInt := new(big.Int).SetBytes(seed[:])
	index = seedInt.Mod(seedInt, big.NewInt(numSegments)).Uint64()
	return
}

// StorageProofSegmentIndex takes a contractID and a windowIndex and calculates
// the index of the segment that should be proven on when doing a proof of
// storage.
func (s *State) StorageProofSegmentIndex(contractID ContractID) (index uint64, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.storageProofSegmentIndex(contractID)
}

// validProof returns err = nil if the storage proof provided is valid given
// the state context, otherwise returning an error to indicate what is invalid.
func (s *State) validProof(sp StorageProof) error {
	contract, exists := s.openContracts[sp.ContractID]
	if !exists {
		return errors.New("unrecognized contract id in storage proof")
	}

	// Check that the storage proof itself is valid.
	segmentIndex, err := s.storageProofSegmentIndex(sp.ContractID)
	if err != nil {
		return err
	}
	if !hash.VerifyReaderProof(
		sp.Segment,
		sp.HashSet,
		hash.CalculateSegments(contract.FileSize),
		segmentIndex,
		contract.FileMerkleRoot,
	) {
		return errors.New("provided storage proof is invalid")
	}

	return nil
}

// applyStorageProof takes a storage proof and adds any outputs created by it
// to the consensus state.
func (s *State) applyStorageProof(sp StorageProof) (od OutputDiff, cd ContractDiff) {
	// Create the output and add it to the list of unspent outputs.
	contract := s.openContracts[sp.ContractID]
	output := Output{
		Value:     contract.Payout,
		SpendHash: contract.ValidProofAddress,
	}
	outputID, err := contract.StorageProofOutputID(sp.ContractID, s.height(), true)
	if err != nil {
		if DEBUG {
			panic(err) // hard to avoid
		}
	}
	s.unspentOutputs[outputID] = output

	// Delete the contract.
	delete(s.openContracts, sp.ContractID)

	od = OutputDiff{
		New:    true,
		ID:     outputID,
		Output: output,
	}
	cd = ContractDiff{
		New:        false,
		ContractID: sp.ContractID,
		Contract:   contract,
	}
	return
}

// validContract returns err = nil if the contract is valid in the current
// context of the state, and returns an error if something about the contract
// is invalid.
func (s *State) validContract(c FileContract) (err error) {
	if c.Start <= s.height() {
		err = errors.New("contract must start in the future.")
		return
	}
	if c.End <= c.Start {
		err = errors.New("contract duration must be at least one block.")
		return
	}

	return
}

// addContract takes a FileContract and its corresponding ContractID and adds
// it to the state.
func (s *State) applyContract(contract FileContract, id ContractID) (cd ContractDiff) {
	s.openContracts[id] = contract

	cd = ContractDiff{
		New:        true,
		ContractID: id,
		Contract:   contract,
	}
	return
}

// applyMissedProof adds outputs to the State to manage a missed storage proof
// on a file contract.
func (s *State) applyMissedProof(openContract *OpenContract) (diff OutputDiff) {
	contract := openContract.FileContract
	payout := contract.Payout

	// Create the output for the missed proof.
	newOutputID, err := contract.StorageProofOutputID(openContract.ContractID, s.height(), false)
	if err != nil {
		panic(err)
	}
	output := Output{
		Value:     payout,
		SpendHash: contract.MissedProofAddress,
	}
	s.unspentOutputs[newOutputID] = output
	msp := MissedStorageProof{
		OutputID:   newOutputID,
		ContractID: openContract.ContractID,
	}
	diff = OutputDiff{New: true, ID: newOutputID, Output: output}

	// Update the open contract to reflect the missed payment.
	s.currentBlockNode().MissedStorageProofs = append(s.currentBlockNode().MissedStorageProofs, msp)
	return
}

// contractMaintenance checks the contract windows and storage proofs and to
// create outputs for missed proofs and contract terminations, and to advance
// any storage proof windows.
//
// TODO: Contracts should terminate immediately...
func (s *State) applyContractMaintenance(bn *BlockNode) {
	// Scan all open contracts and perform any required maintenance on each.
	var contractsToDelete []ContractID
	for _, openContract := range s.openContracts {
		// Check if the contract has ended.
		contract := openContract.FileContract
		if s.height() == contract.End {
			// If the proof was missed for this window, add an output.
			cd := ContractDiff{
				Contract:   openContract.FileContract,
				ContractID: openContract.ContractID,
				New:        false,
			}
			diff := s.applyMissedProof(openContract)
			diffs = append(diffs, diff)
		}

		contractsToDelete = append(contractsToDelete, openContract.ContractID)
	}

	// Delete all of the contracts that terminated.
	for _, contractID := range contractsToDelete {
		delete(s.openContracts, contractID)
	}
	return
}

// inverseContractMaintenance does the inverse of contract maintenance, moving
// the state of contracts backwards instead forwards.
func (s *State) invertContractMaintenance() (diffs []OutputDiff) {
	// function unsalvageable.
	return
}
