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

		scod := SiacoinOutputDiff{
			Direction:     DiffApply,
			ID:            id,
			SiacoinOutput: sco,
		}
		s.siacoinOutputs[id] = sco
		bn.siacoinOutputDiffs = append(bn.siacoinOutputDiffs, scod)
	}
}

// applyMissedProof adds the outputs and diffs that result from a contract
// expiring.
func (s *State) applyMissedProof(bn *blockNode, fcid FileContractID) {
	// Sanity check - the id must correspond to an existing contract.
	fc, exists := s.fileContracts[fcid]
	if !exists {
		if DEBUG {
			panic("misuse of applyMissedProof")
		}
		return
	}

	// Add all of the outputs in the missed proof outputs to the consensus set.
	for i, output := range fc.MissedProofOutputs {
		// Sanity check - output should not already exist.
		outputID := fcid.StorageProofOutputID(false, i)
		if DEBUG {
			_, exists := s.delayedSiacoinOutputs[s.height()][outputID]
			if exists {
				panic("missed proof output already exists in the delayed outputs set")
			}
			_, exists = s.siacoinOutputs[outputID]
			if exists {
				panic("missed proof output already exists in the siacoin outputs set")
			}
		}

		bn.delayedSiacoinOutputs[outputID] = output
		s.delayedSiacoinOutputs[s.height()][outputID] = output
	}

	// Remove the file contract from the consensus set and update the diffs.
	fcd := FileContractDiff{
		Direction:    DiffRevert,
		ID:           fcid,
		FileContract: fc,
	}
	delete(s.fileContracts, fcid)
	bn.fileContractDiffs = append(bn.fileContractDiffs, fcd)

	return
}

// applyContractMaintenance iterates through all of the contracts in the
// consensus set and calls 'applyMissedProof' on any that have expired.
func (s *State) applyContractMaintenance(bn *blockNode) {
	// Iterate through all contracts and figure out which ones have expired.
	// Expiring a contract deletes it from the map we are iterating through, so
	// we need to store it and deleted once we're done iterating through the
	// map.
	var expiredContracts []FileContractID
	for id, contract := range s.fileContracts {
		if s.height() == contract.Expiration {
			expiredContracts = append(expiredContracts, id)
		}
	}

	// Handle all of the contracts that have expired.
	for _, id := range expiredContracts {
		s.applyMissedProof(bn, id)
	}

	return
}

// applyMaintence generates, adds, and applies diffs that are generated after
// all of the transactions of a block have been processed. This includes adding
// the miner susidies, adding any matured outputs to the set of siacoin
// outputs, and dealing with any contracts that have expired.
func (s *State) applyMaintenance(bn *blockNode) {
	s.applyMinerSubsidy(bn)
	s.applyDelayedSiacoinOutputMaintenance(bn)
	s.applyContractMaintenance(bn)
}
