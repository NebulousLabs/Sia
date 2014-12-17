package consensus

import (
	"errors"
	"math/big"

	"github.com/NebulousLabs/Sia/hash"
)

// StorageProofSegmentIndex takes a contractID and a windowIndex and calculates
// the index of the segment that should be proven on when doing a proof of
// storage.
func (s *State) StorageProofSegmentIndex(contractID ContractID, windowIndex BlockHeight) (index uint64, err error) {
	openContract, exists := s.openContracts[contractID]
	if !exists {
		err = errors.New("urecognized contractID")
		return
	}
	contract := openContract.FileContract

	// Get random number seed used to pick the index.
	triggerBlockHeight := contract.Start + contract.ChallengeWindow*windowIndex - 1
	triggerBlock, err := s.BlockAtHeight(triggerBlockHeight)
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
	segmentIndex, err := s.StorageProofSegmentIndex(sp.ContractID, sp.WindowIndex)
	if err != nil {
		return err
	}
	if !hash.VerifyReaderProof(
		sp.Segment,
		sp.HashSet,
		hash.CalculateSegments(openContract.FileContract.FileSize),
		segmentIndex,
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
	if c.Start <= s.Height() {
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
		WindowSatisfied: false,
	}
}

// recordMissedProof adds outputs to the State to manage a missed storage proof
// on a file contract.
func (s *State) recordMissedProof(openContract *OpenContract) {
	contract := openContract.FileContract
	payout := contract.MissedProofPayout
	if openContract.FundsRemaining < contract.MissedProofPayout {
		payout = openContract.FundsRemaining
	}

	// Create the output for the missed proof.
	newOutputID, err := contract.StorageProofOutputID(openContract.ContractID, s.Height(), false)
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

	// Update the open contract to reflect the missed payment.
	s.currentBlockNode().MissedStorageProofs = append(s.currentBlockNode().MissedStorageProofs, msp)
	openContract.FundsRemaining -= payout
	openContract.Failures += 1
}

// contractMaintenance checks the contract windows and storage proofs and to
// create outputs for missed proofs and contract terminations, and to advance
// any storage proof windows.
func (s *State) contractMaintenance() {
	// Scan all open contracts and perform any required maintenance on each.
	var contractsToDelete []ContractID
	for _, openContract := range s.openContracts {
		// Check if the window index is changing.
		contract := openContract.FileContract
		contractProgress := s.Height() - contract.Start
		if s.Height() > contract.Start && contractProgress%contract.ChallengeWindow == 0 {
			if openContract.WindowSatisfied == false {
				s.recordMissedProof(openContract)
			} else {
				s.currentBlockNode().SuccessfulWindows = append(s.currentBlockNode().SuccessfulWindows, openContract.ContractID)
			}
			openContract.WindowSatisfied = false
		}

		// Check for a terminated contract.
		if openContract.FundsRemaining == 0 || contract.End == s.Height() || contract.Tolerance == openContract.Failures {
			if openContract.FundsRemaining != 0 {
				// Create a new output that terminates the contract.
				output := Output{
					Value: openContract.FundsRemaining,
				}

				// Get the output address.
				contractSuccess := openContract.Failures != openContract.FileContract.Tolerance
				if contractSuccess {
					output.SpendHash = contract.ValidProofAddress
				} else {
					output.SpendHash = contract.MissedProofAddress
				}

				// Create the output.
				outputID := ContractTerminationOutputID(openContract.ContractID, contractSuccess)
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
