package sia

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
