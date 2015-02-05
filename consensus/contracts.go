package consensus

import (
	"errors"
	"math/big"

	"github.com/NebulousLabs/Sia/hash"
)

func (s *State) storageProofSegment(fcid FileContractID) (index uint64, err error) {
	contract, exists := s.openFileContracts[fcid]
	if !exists {
		err = errors.New("unrecognized file contract id")
		return
	}

	triggerHeight := contract.Start - 1
	triggerBlock, exists := s.blockAtHeight(triggerHeight)
	if !exists {
		err = errors.New("no block found at contract trigger block height")
		return
	}
	triggerID := triggerBlock.ID()

	seed := hash.HashBytes(append(triggerID[:], fcid[:]...))
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
	contract, exists := s.openFileContracts[sp.FileContractID]
	if !exists {
		return errors.New("unrecognized contract id in storage proof")
	}

	// Check that the storage proof itself is valid.
	segmentIndex, err := s.storageProofSegment(sp.FileContractID)
	if err != nil {
		return err
	}
	verified := hash.VerifySegment(
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
func (s *State) applyContract(fc FileContract, fcid FileContractID) (fcd FileContractDiff) {
	s.openFileContracts[fcid] = fc
	fcd = FileContractDiff{
		New:          true,
		ID:           fcid,
		FileContract: fc,
	}
	return
}

// applyStorageProof takes a storage proof and adds any outputs created by it
// to the consensus state.
func (s *State) applyStorageProof(sp StorageProof) (scod SiacoinOutputDiff, fcd FileContractDiff) {
	// Calculate the new output and its id.
	contract := s.openFileContracts[sp.FileContractID]
	sco := SiacoinOutput{
		Value:     contract.Payout,
		SpendHash: contract.ValidProofAddress,
	}
	outputID := sp.FileContractID.StorageProofOutputID(true)

	// Update the state.
	s.unspentSiacoinOutputs[outputID] = sco
	delete(s.openFileContracts, sp.FileContractID)

	scod = SiacoinOutputDiff{
		New:           true,
		ID:            outputID,
		SiacoinOutput: sco,
	}
	fcd = FileContractDiff{
		New:          false,
		ID:           sp.FileContractID,
		FileContract: contract,
	}
	return
}

// applyMissedProof adds outputs to the State to manage a missed storage proof
// on a file contract.
func (s *State) applyMissedProof(fc FileContract, fcid FileContractID) (scod SiacoinOutputDiff, fcd FileContractDiff) {
	// Create the output for the missed proof.
	sco := SiacoinOutput{
		Value:     fc.Payout,
		SpendHash: fc.MissedProofAddress,
	}
	outputID := fcid.StorageProofOutputID(false)

	// Update the state.
	s.unspentSiacoinOutputs[outputID] = sco
	delete(s.openFileContracts, fcid)

	// Create the diffs.
	fcd = FileContractDiff{
		New:          false,
		ID:           fcid,
		FileContract: fc,
	}
	scod = SiacoinOutputDiff{
		New:           true,
		ID:            outputID,
		SiacoinOutput: sco,
	}
	return
}

func (s *State) applyContractMaintenance() (scods []SiacoinOutputDiff, fcds []FileContractDiff) {
	// Iterate through all contracts and figure out which ones have expired.
	// Expiring a contract deletes it from the map we are iterating through, so
	// we need to store it and deleted once we're done iterating through the
	// map.
	var expiredContracts []FileContractID
	for id, contract := range s.openFileContracts {
		if s.height() == contract.End {
			expiredContracts = append(expiredContracts, id)
		}
	}

	// Delete all of the contracts that terminated.
	for _, id := range expiredContracts {
		contract := s.openFileContracts[id]
		scod, fcd := s.applyMissedProof(contract, id)
		scods = append(scods, scod)
		fcds = append(fcds, fcd)
	}

	return
}

// StorageProofSegmentIndex takes a contractID and a windowIndex and calculates
// the index of the segment that should be proven on when doing a proof of
// storage.
func (s *State) StorageProofSegment(fcid FileContractID) (index uint64, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.storageProofSegment(fcid)
}

func (s *State) ValidContract(fc FileContract) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.validContract(fc)
}
