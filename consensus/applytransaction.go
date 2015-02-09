package consensus

// applySiacoinInputs takes all of the siacoin inputs in a transaction and
// applies them to the state, updating the diffs in the block node.
func (s *State) applySiacoinInputs(bn *blockNode, t Transaction) {
	// Remove all siacoin inputs from the unspent siacoin outputs list.
	for _, sci := range t.SiacoinInputs {
		// Sanity check - the input should exist within the blockchain.
		if DEBUG {
			_, exists := s.siacoinOutputs[sci.ParentID]
			if !exists {
				panic("Applying a transaction with an invalid unspent output!")
			}
		}

		scod := SiacoinOutputDiff{
			New:           false,
			ID:            sci.ParentID,
			SiacoinOutput: s.siacoinOutputs[sci.ParentID],
		}
		delete(s.siacoinOutputs, sci.ParentID)
		bn.siacoinOutputDiffs = append(bn.siacoinOutputDiffs, scod)
	}
}

// applySiacoinOutputs takes all of the siacoin outputs in a transaction and
// applies them to the state, updating the diffs in the block node.
func (s *State) applySiacoinOutputs(bn *blockNode, t Transaction) {
	// Add all siacoin outputs to the unspent siacoin outputs list.
	for i, sco := range t.SiacoinOutputs {
		// Sanity check - the output should not exist within the state.
		scoid := t.SiacoinOutputID(i)
		if DEBUG {
			_, exists := s.siacoinOutputs[scoid]
			if exists {
				panic("applying a siacoin output when the output already exists")
			}
		}

		scod := SiacoinOutputDiff{
			New:           true,
			ID:            scoid,
			SiacoinOutput: sco,
		}
		s.siacoinOutputs[scoid] = sco
		bn.siacoinOutputDiffs = append(bn.siacoinOutputDiffs, scod)
	}
}

// applyFileContracts iterates through all of the file contracts in a
// transaction and applies them to the state, updating the diffs in the block
// node.
func (s *State) applyFileContracts(bn *blockNode, t Transaction) {
	for i, fc := range t.FileContracts {
		// Sanity check - the file contract should not exists within the state.
		fcid := t.FileContractID(i)
		if DEBUG {
			_, exists := s.fileContracts[fcid]
			if exists {
				panic("applying a file contract when the contract already exists")
			}
		}

		fcd := FileContractDiff{
			New:          true,
			ID:           fcid,
			FileContract: fc,
		}
		s.fileContracts[fcid] = fc
		bn.fileContractDiffs = append(bn.fileContractDiffs, fcd)
	}
	return
}

// applyFileContractTerminations iterates through all of the file contract
// terminations in a transaction and applies them to the state, updating the
// diffs in the block node.
func (s *State) applyFileContractTerminations(bn *blockNode, t Transaction) {
	for _, fct := range t.FileContractTerminations {
		// Sanity check - termination should affect an existing contract.
		fc, exists := s.fileContracts[fct.ParentID]
		if !exists {
			if DEBUG {
				panic("file contract termination terminates a nonexisting contract")
			}
			continue
		}

		// Add the diff for the deletion to the block node.
		fcd := FileContractDiff{
			New:          false,
			ID:           fct.ParentID,
			FileContract: fc,
		}
		delete(s.fileContracts, fct.ParentID)
		bn.fileContractDiffs = append(bn.fileContractDiffs, fcd)

		// Add all of the payouts to the consensus set and block node diffs.
		for i, payout := range fct.Payouts {
			id := fct.ParentID.FileContractTerminationPayoutID(i)
			s.delayedSiacoinOutputs[s.height()][id] = payout
			bn.delayedSiacoinOutputs[id] = payout
		}
	}
}

// applyStorageProofs iterates through all of the storage proofs in a
// transaction and applies them to the state, updating the diffs in the block
// node.
func (s *State) applyStorageProofs(bn *blockNode, t Transaction) {
	for _, sp := range t.StorageProofs {
		// Sanity check - output should not already exist.
		outputID := sp.ParentID.StorageProofOutputID(true)
		if DEBUG {
			_, exists := s.siacoinOutputs[outputID]
			if exists {
				panic("storage proof output already exists")
			}
		}

		// Get the portion of the payout that goes into the siafundPool and the
		// portion that goes into the siacoin output created for the storage
		// proof, and then create the siacoin output and add the pool portion
		// to the pool.
		fc := s.fileContracts[sp.ParentID]
		poolPortion, outputPortion := splitContractPayout(fc.Payout)
		sco := SiacoinOutput{
			Value:      outputPortion,
			UnlockHash: fc.ValidProofUnlockHash,
		}
		err := s.siafundPool.Add(poolPortion)
		if DEBUG {
			if err != nil {
				panic(err)
			}
		}

		// Remove the contract from the consensus set, add the output to the
		// consensus set, and update the diffs to reflect each change.
		fcd := FileContractDiff{
			New:          false,
			ID:           sp.ParentID,
			FileContract: fc,
		}
		delete(s.fileContracts, sp.ParentID)
		s.delayedSiacoinOutputs[s.height()][outputID] = sco
		bn.delayedSiacoinOutputs[outputID] = sco
		bn.fileContractDiffs = append(bn.fileContractDiffs, fcd)
	}
	return
}

// applySiafundInputs takes all of the siafund inputs in a transaction and
// applies them to the state, updating the diffs in the block node.
func (s *State) applySiafundInputs(bn *blockNode, t Transaction) {
	for _, sfi := range t.SiafundInputs {
		// Sanity check - the input should exist within the blockchain.
		if DEBUG {
			_, exists := s.siafundOutputs[sfi.ParentID]
			if !exists {
				panic("applying a transaction with an invalid unspent siafund output")
			}
			continue
		}

		// Calculate the volume of siacoins to put in the claim output.
		claimPortion := s.siafundPool
		sfo := s.siafundOutputs[sfi.ParentID]
		err := claimPortion.Sub(sfo.ClaimStart)
		if err != nil {
			if DEBUG {
				panic("error while handling claim portion")
			}
			continue
		}
		err = claimPortion.Div(NewCurrency64(SiafundCount))
		if err != nil {
			if DEBUG {
				panic("error while handling claim portion")
			}
			continue
		}

		// Add the claim output to the delayed set of outputs.
		sco := SiacoinOutput{
			Value:      claimPortion,
			UnlockHash: sfo.ClaimUnlockHash,
		}
		scoid := sfi.ParentID.SiaClaimOutputID()
		s.delayedSiacoinOutputs[s.height()][scoid] = sco
		bn.delayedSiacoinOutputs[scoid] = sco

		// Create the siafund output diff and remove the output from the
		// consensus set.
		sfod := SiafundOutputDiff{
			New:           false,
			ID:            sfi.ParentID,
			SiafundOutput: s.siafundOutputs[sfi.ParentID],
		}
		delete(s.siafundOutputs, sfi.ParentID)
		bn.siafundOutputDiffs = append(bn.siafundOutputDiffs, sfod)
	}
}

// applySiafundOutputs takes all of the siafund outputs in a transaction and
// applies them to the state, updating the diffs in the block node.
func (s *State) applySiafundOutputs(bn *blockNode, t Transaction) {
	for i, sfo := range t.SiafundOutputs {
		// Sanity check - the output should not exist within the blockchain.
		sfoid := t.SiafundOutputID(i)
		if DEBUG {
			_, exists := s.siafundOutputs[sfoid]
			if exists {
				panic("siafund being added to consensus set when it is already in the consensus set")
			}
		}

		// Set the claim start.
		sfo.ClaimStart = s.siafundPool

		// Create and apply the diff.
		sfod := SiafundOutputDiff{
			New:           true,
			ID:            sfoid,
			SiafundOutput: sfo,
		}
		s.siafundOutputs[sfoid] = sfo
		bn.siafundOutputDiffs = append(bn.siafundOutputDiffs, sfod)
	}
}

// applyTransaction takes a transaction and uses the contents to update the
// state of consensus according to the contents of the transaction. The
// transaction is assumed to be valid. A set of diffs are returned that
// represent how the state of consensus has changed. The changes to the
// siafundPool and the delayedSiacoinOutputs are not recorded, as they are
// handled externally.
func (s *State) applyTransaction(bn *blockNode, t Transaction) {
	// Sanity check - the input transaction should be valid.
	if DEBUG {
		err := s.validTransaction(t)
		if err != nil {
			panic("applyTransaction called with an invalid transaction!")
		}
	}

	// Apply each component of the transaction. Miner fees are handled as a
	// separate process.
	s.applySiacoinInputs(bn, t)
	s.applySiacoinOutputs(bn, t)
	s.applyFileContracts(bn, t)
	s.applyFileContractTerminations(bn, t)
	s.applyStorageProofs(bn, t)
	s.applySiafundInputs(bn, t)
	s.applySiafundOutputs(bn, t)
}
