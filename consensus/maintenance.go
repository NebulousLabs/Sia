package consensus

// applyMinerSubsidy adds all of the outputs recorded in the MinerPayouts to
// the state, and returns the corresponding set of diffs.
func (s *State) applyMinerSubsidy(bn *blockNode) {
	for i, payout := range bn.block.MinerPayouts {
		// Sanity check - the output should not already be in
		// delayedSiacoinOutputs, and should also not be in siacoinOutputs.
		id := bn.block.MinerPayoutID(i)
		if DEBUG {
			_, exists := s.delayedSiacoinOutputs[s.height()][id]
			if exists {
				panic("miner subsidy already in delayed outputs")
			}
			_, exists = s.siacoinOutputs[id]
			if exists {
				panic("miner subsidy already in siacoin outputs")
			}
		}

		s.delayedSiacoinOutputs[s.height()][id] = payout
		bn.delayedSiacoinOutputs[id] = payout
	}
	return
}

// applyDelayedSiacoinOutputMaintenance goes through all of the outputs that
// have matured and adds them to the list of siacoinOutputs.
func (s *State) applyDelayedSiacoinOutputMaintenance(bn *blockNode) {
	for id, sco := range s.delayedSiacoinOutputs[bn.height-MaturityDelay] {
		// Sanity check - the output should not already be in siacoinOuptuts.
		if DEBUG {
			_, exists := s.siacoinOutputs[id]
			if exists {
				panic("trying to add a delayed output when the output is already there")
			}
		}
		s.siacoinOutputs[id] = sco

		// Add the matured outputs as a diff of this block.
		scod := SiacoinOutputDiff{
			New:           true,
			ID:            id,
			SiacoinOutput: sco,
		}
		bn.siacoinOutputDiffs = append(bn.siacoinOutputDiffs, scod)
	}
}

// applyMissedProof adds outputs to the State to manage a missed storage proof
// on a file contract.
func (s *State) applyMissedProof(bn *blockNode, fc FileContract, fcid FileContractID) {
	// Get the portion of the payout that goes to the siafundPool, and the
	// portion of the payout that goes to the missed proof output.
	poolPortion, outputPortion := splitContractPayout(fc.Payout)

	// Create the output for the missed proof.
	sco := SiacoinOutput{
		Value:      outputPortion,
		UnlockHash: fc.MissedProofUnlockHash,
	}
	outputID := fcid.StorageProofOutputID(false)

	// Update the state to include the storage proof output (which goes into
	// the delayed set) and the siafund pool.
	s.delayedSiacoinOutputs[s.height()][outputID] = sco
	delete(s.fileContracts, fcid)
	err := s.siafundPool.Add(poolPortion)
	if DEBUG {
		if err != nil {
			panic(err)
		}
	}

	// Create the diffs.
	fcd := FileContractDiff{
		New:          false,
		ID:           fcid,
		FileContract: fc,
	}
	bn.fileContractDiffs = append(bn.fileContractDiffs, fcd)
	bn.delayedSiacoinOutputs[outputID] = sco
	return
}

// TODO: check out this whole function.
func (s *State) applyContractMaintenance(bn *blockNode) {
	// Iterate through all contracts and figure out which ones have expired.
	// Expiring a contract deletes it from the map we are iterating through, so
	// we need to store it and deleted once we're done iterating through the
	// map.
	var expiredContracts []FileContractID
	for id, contract := range s.fileContracts {
		if s.height() == contract.End {
			expiredContracts = append(expiredContracts, id)
		}
	}

	// Delete all of the contracts that terminated.
	for _, id := range expiredContracts {
		contract := s.fileContracts[id]
		s.applyMissedProof(bn, contract, id)
	}

	return
}

func (s *State) applyMaintenance(bn *blockNode) {
	s.applyMinerSubsidy(bn)
	s.applyDelayedSiacoinOutputMaintenance(bn)
	s.applyContractMaintenance(bn)
}
