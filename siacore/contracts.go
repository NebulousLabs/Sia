package siacore

import (
	"errors"
	"math/big"

	"github.com/NebulousLabs/Andromeda/hash"
)

// currentProofIndex returns the index that should be used when building and
// verifying the storage proof for a file at the given window.
func (s *State) currentProofIndex(sp StorageProof) (proofIndex uint64) {
	contract := s.openContracts[sp.ContractID].FileContract

	windowIndex, err := contract.WindowIndex(s.Height())
	if err != nil {
		return
	}
	triggerBlock := windowIndex*contract.Start - 1
	triggerBlockID := s.currentPath[triggerBlock]

	indexSeed := hash.HashBytes(append(triggerBlockID[:], sp.ContractID[:]...))
	seedInt := new(big.Int).SetBytes(indexSeed[:])
	modSeed := seedInt.Mod(seedInt, big.NewInt(int64(contract.FileSize)))

	return modSeed.Uint64()
}

// validProof returns err = nil if the storage proof provided is valid given
// the state context, otherwise returning an error to indicate what is invalid.
func (s *State) validProof(sp StorageProof) error {
	openContract, exists := s.openContracts[sp.ContractID]
	if !exists {
		return errors.New("unrecognized contract id in storage proof")
	}

	// Check that the proof has not already been submitted.
	if openContract.WindowSatisfied {
		return errors.New("storage proof has already been completed for this contract")
	}

	// Check that the storage proof itself is valid.
	if !hash.VerifyReaderProof(
		sp.Segment,
		sp.HashSet,
		hash.CalculateSegments(openContract.FileContract.FileSize),
		s.currentProofIndex(sp),
		openContract.FileContract.FileMerkleRoot,
	) {
		return errors.New("provided storage proof is invalid")
	}

	return nil
}

// applyStorageProof takes a storage proof and adds any outputs created by it
// to the consensus state.
func (s *State) applyStorageProof(sp StorageProof) {
	// Set the payout of the output - payout cannot be greater than the
	// amount of funds remaining.
	openContract := s.openContracts[sp.ContractID]
	payout := openContract.FileContract.ValidProofPayout
	if openContract.FundsRemaining < openContract.FileContract.ValidProofPayout {
		payout = openContract.FundsRemaining
	}

	// Create the output and add it to the list of unspent outputs.
	output := Output{
		Value:     payout,
		SpendHash: openContract.FileContract.ValidProofAddress,
	}
	outputID, err := openContract.FileContract.StorageProofOutputID(openContract.ContractID, s.Height(), true)
	if err != nil {
		panic(err)
	}
	s.unspentOutputs[outputID] = output

	// Mark the proof as complete for this window, and subtract from the
	// FundsRemaining.
	s.openContracts[sp.ContractID].WindowSatisfied = true
	s.openContracts[sp.ContractID].FundsRemaining -= payout
}

// validContract returns err = nil if the contract is valid in the current
// context of the state, and returns an error if something about the contract
// is invalid.
func (s *State) validContract(c FileContract) (err error) {
	if c.ContractFund < 0 {
		err = errors.New("contract must be funded.")
		return
	}
	if c.Start < s.Height() {
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
func (s *State) addContract(contract FileContract, id ContractID) {
	s.openContracts[id] = &OpenContract{
		FileContract:    contract,
		ContractID:      id,
		FundsRemaining:  contract.ContractFund,
		Failures:        0,
		WindowSatisfied: true, // The first window is free, because the start is in the future by mandate.
	}
}

// contractMaintenance checks the contract windows and storage proofs and to
// create outputs for missed proofs and contract terminations, and to advance
// any storage proof windows.
func (s *State) contractMaintenance() {
	// Scan all open contracts and perform any required maintenance on each.
	var contractsToDelete []ContractID
	for _, openContract := range s.openContracts {
		// Check for the window switching over.
		if (s.Height()-openContract.FileContract.Start)%openContract.FileContract.ChallengeFrequency == 0 && s.Height() > openContract.FileContract.Start {
			// Check for a missed proof.
			if openContract.WindowSatisfied == false {
				// Determine payout of missed proof.
				payout := openContract.FileContract.MissedProofPayout
				if openContract.FundsRemaining < openContract.FileContract.MissedProofPayout {
					payout = openContract.FundsRemaining
				}

				// Create the output for the missed proof.
				newOutputID, err := openContract.FileContract.StorageProofOutputID(openContract.ContractID, s.Height(), false)
				if err != nil {
					panic(err)
				}
				output := Output{
					Value:     payout,
					SpendHash: openContract.FileContract.MissedProofAddress,
				}
				s.unspentOutputs[newOutputID] = output
				msp := MissedStorageProof{
					OutputID:   newOutputID,
					ContractID: openContract.ContractID,
				}
				s.currentBlockNode().MissedStorageProofs = append(s.currentBlockNode().MissedStorageProofs, msp)

				// Update the FundsRemaining
				openContract.FundsRemaining -= payout

				// Update the failures count.
				openContract.Failures += 1
			} else {
				s.currentBlockNode().SuccessfulWindows = append(s.currentBlockNode().SuccessfulWindows, openContract.ContractID)
			}
			openContract.WindowSatisfied = false
		}

		// Check for a terminated contract.
		if openContract.FundsRemaining == 0 || openContract.FileContract.End == s.Height() || openContract.FileContract.Tolerance == openContract.Failures {
			if openContract.FundsRemaining != 0 {
				// Create a new output that terminates the contract.
				contractStatus := openContract.Failures == openContract.FileContract.Tolerance
				outputID := ContractTerminationOutputID(openContract.ContractID, contractStatus)
				output := Output{
					Value: openContract.FundsRemaining,
				}
				if openContract.FileContract.Tolerance == openContract.Failures {
					output.SpendHash = openContract.FileContract.MissedProofAddress
				} else {
					output.SpendHash = openContract.FileContract.ValidProofAddress
				}
				s.unspentOutputs[outputID] = output
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
		delete(s.openContracts, contractID)
	}
}

// inverseContractMaintenance does the inverse of contract maintenance, moving
// the state of contracts backwards instead forwards.
func (s *State) inverseContractMaintenance() {
	// Repen all contracts that terminated, and remove the corresponding output.
	for _, openContract := range s.currentBlockNode().ContractTerminations {
		s.openContracts[openContract.ContractID] = openContract
		contractStatus := openContract.Failures == openContract.FileContract.Tolerance
		delete(s.unspentOutputs, ContractTerminationOutputID(openContract.ContractID, contractStatus))
	}

	// Reverse all outputs created by missed storage proofs.
	for _, missedProof := range s.currentBlockNode().MissedStorageProofs {
		s.openContracts[missedProof.ContractID].FundsRemaining += s.unspentOutputs[missedProof.OutputID].Value
		s.openContracts[missedProof.ContractID].Failures -= 1
		delete(s.unspentOutputs, missedProof.OutputID)
	}

	// Reset the window satisfied variable to true for all successful windows.
	for _, id := range s.currentBlockNode().SuccessfulWindows {
		s.openContracts[id].WindowSatisfied = true
	}
}
