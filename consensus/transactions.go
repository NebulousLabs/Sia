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
		bn.siacoinOutputDiffs = append(bn.siacoinOutputDiffs, scod)
		delete(s.siacoinOutputs, sci.ParentID)
	}
}

// applySiacoinOutputs takes all of the siacoin outputs in a transaction and
// applies them to the state, updating the diffs in the block node.
func (s *State) applySiacoinOutputs(bn *blockNode, t Transaction) {
	// Add all siacoin outputs to the unspent siacoin outputs list.
	for i, sco := range t.SiacoinOutputs {
		// Sanity check - the output should not exist within the state.
		if DEBUG {
			_, exists := s.siacoinOutputs[t.SiacoinOutputID(i)]
			if exists {
				panic("applying a  transaction with an invalid new output")
			}
		}

		scod := SiacoinOutputDiff{
			New:           true,
			ID:            t.SiacoinOutputID(i),
			SiacoinOutput: sco,
		}
		s.siacoinOutputs[t.SiacoinOutputID(i)] = sco
		bn.siacoinOutputDiffs = append(bn.siacoinOutputDiffs, scod)
	}
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
		}

		// Calculate the volume of siacoins to put in the claim output.
		claimPortion := s.siafundPool
		sfo := s.siafundOutputs[sfi.ParentID]
		err := claimPortion.Sub(sfo.ClaimStart)
		if err != nil {
			if DEBUG {
				panic("error while handling claim portion")
			} else {
				continue
			}
		}
		err = claimPortion.Div(NewCurrency64(SiafundCount))
		if err != nil {
			if DEBUG {
				panic("error while handling claim portion")
			} else {
				continue
			}
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
		bn.siafundOutputDiffs = append(bn.siafundOutputDiffs, sfod)
		delete(s.siafundOutputs, sfi.ParentID)
	}
}

// applySiafundOutputs takes all of the siafund outputs in a transaction and
// applies them to the state, updating the diffs in the block node.
func (s *State) applySiafundOutputs(bn *blockNode, t Transaction) {
	for i, sfo := range t.SiafundOutputs {
		// Set the claim start.
		sfo.ClaimStart = s.siafundPool

		// Create and apply the diff.
		sfod := SiafundOutputDiff{
			New:           true,
			ID:            t.SiafundOutputID(i),
			SiafundOutput: sfo,
		}
		s.siafundOutputs[t.SiafundOutputID(i)] = sfo
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
