package sia

import (
	"errors"
)

// applyStorageProof takes a storage proof and adds any outputs created by it
// to the consensus state.
func (s *State) applyStorageProof(sp StorageProof) {
	// Set the payout of the output - payout cannot be greater than the
	// amount of funds remaining.
	openContract := s.OpenContracts[sp.ContractID]
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
	s.UnspentOutputs[outputID] = output

	// Mark the proof as complete for this window, and subtract from the
	// FundsRemaining.
	s.OpenContracts[sp.ContractID].WindowSatisfied = true
	s.OpenContracts[sp.ContractID].FundsRemaining -= payout
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
	openContract := OpenContract{
		FileContract:    contract,
		ContractID:      id,
		FundsRemaining:  contract.ContractFund,
		Failures:        0,
		WindowSatisfied: true, // The first window is free, because the start is in the future by mandate.
	}
	s.OpenContracts[id] = &openContract
}
