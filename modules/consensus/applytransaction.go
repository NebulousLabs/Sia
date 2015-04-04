package consensus

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// applySiacoinInputs takes all of the siacoin inputs in a transaction and
// applies them to the state, updating the diffs in the block node.
func (s *State) applySiacoinInputs(bn *blockNode, t types.Transaction) {
	// Remove all siacoin inputs from the unspent siacoin outputs list.
	for _, sci := range t.SiacoinInputs {
		// Sanity check - the input should exist within the blockchain.
		if build.DEBUG {
			_, exists := s.siacoinOutputs[sci.ParentID]
			if !exists {
				panic("Applying a transaction with an invalid unspent output!")
			}
		}

		bn.siacoinOutputDiffs = append(bn.siacoinOutputDiffs, modules.SiacoinOutputDiff{
			Direction:     modules.DiffRevert,
			ID:            sci.ParentID,
			SiacoinOutput: s.siacoinOutputs[sci.ParentID],
		})
		delete(s.siacoinOutputs, sci.ParentID)
	}
}

// applySiacoinOutputs takes all of the siacoin outputs in a transaction and
// applies them to the state, updating the diffs in the block node.
func (s *State) applySiacoinOutputs(bn *blockNode, t types.Transaction) {
	// Add all siacoin outputs to the unspent siacoin outputs list.
	for i, sco := range t.SiacoinOutputs {
		// Sanity check - the output should not exist within the state.
		scoid := t.SiacoinOutputID(i)
		if build.DEBUG {
			_, exists := s.siacoinOutputs[scoid]
			if exists {
				panic("applying a siacoin output when the output already exists")
			}
		}

		bn.siacoinOutputDiffs = append(bn.siacoinOutputDiffs, modules.SiacoinOutputDiff{
			Direction:     modules.DiffApply,
			ID:            scoid,
			SiacoinOutput: sco,
		})
		s.siacoinOutputs[scoid] = sco
	}
}

// applyFileContracts iterates through all of the file contracts in a
// transaction and applies them to the state, updating the diffs in the block
// node.
func (s *State) applyFileContracts(bn *blockNode, t types.Transaction) {
	for i, fc := range t.FileContracts {
		// Sanity check - the file contract should not exists within the state.
		fcid := t.FileContractID(i)
		if build.DEBUG {
			_, exists := s.fileContracts[fcid]
			if exists {
				panic("applying a file contract when the contract already exists")
			}
		}

		bn.fileContractDiffs = append(bn.fileContractDiffs, modules.FileContractDiff{
			Direction:    modules.DiffApply,
			ID:           fcid,
			FileContract: fc,
		})
		s.fileContracts[fcid] = fc
	}
	return
}

// applyFileContractTerminations iterates through all of the file contract
// terminations in a transaction and applies them to the state, updating the
// diffs in the block node.
func (s *State) applyFileContractTerminations(bn *blockNode, t types.Transaction) {
	for _, fct := range t.FileContractTerminations {
		// Sanity check - termination should affect an existing contract.
		fc, exists := s.fileContracts[fct.ParentID]
		if !exists {
			if build.DEBUG {
				panic("file contract termination terminates a nonexisting contract")
			}
			continue
		}

		// Add the diff for the deletion to the block node.
		bn.fileContractDiffs = append(bn.fileContractDiffs, modules.FileContractDiff{
			Direction:    modules.DiffRevert,
			ID:           fct.ParentID,
			FileContract: fc,
		})
		delete(s.fileContracts, fct.ParentID)

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
func (s *State) applyStorageProofs(bn *blockNode, t types.Transaction) {
	for _, sp := range t.StorageProofs {
		// Sanity check - the file contract of the storage proof should exist.
		fc, exists := s.fileContracts[sp.ParentID]
		if !exists {
			if build.DEBUG {
				panic("storage proof submitted for a file contract that doesn't exist?")
			}
			continue
		}

		// Get the portion of the contract that goes into the siafund pool and
		// add it to the siafund pool.
		s.siafundPool = s.siafundPool.Add(fc.Tax())

		// Add all of the outputs in the ValidProofOutputs of the contract.
		for i, output := range fc.ValidProofOutputs {
			// Sanity check - output should not already exist.
			id := sp.ParentID.StorageProofOutputID(true, i)
			if build.DEBUG {
				_, exists := s.siacoinOutputs[id]
				if exists {
					panic("storage proof output already exists")
				}
			}

			s.delayedSiacoinOutputs[s.height()][id] = output
			bn.delayedSiacoinOutputs[id] = output
		}

		bn.fileContractDiffs = append(bn.fileContractDiffs, modules.FileContractDiff{
			Direction:    modules.DiffRevert,
			ID:           sp.ParentID,
			FileContract: fc,
		})
		delete(s.fileContracts, sp.ParentID)
	}
	return
}

// applySiafundInputs takes all of the siafund inputs in a transaction and
// applies them to the state, updating the diffs in the block node.
func (s *State) applySiafundInputs(bn *blockNode, t types.Transaction) {
	for _, sfi := range t.SiafundInputs {
		// Sanity check - the input should exist within the blockchain.
		if build.DEBUG {
			_, exists := s.siafundOutputs[sfi.ParentID]
			if !exists {
				panic("applying a transaction with an invalid unspent siafund output")
			}
			continue
		}

		// Calculate the volume of siacoins to put in the claim output.
		sfo := s.siafundOutputs[sfi.ParentID]
		claimPortion := s.siafundPool.Sub(sfo.ClaimStart).Div(types.NewCurrency64(types.SiafundCount))

		// Add the claim output to the delayed set of outputs.
		sco := types.SiacoinOutput{
			Value:      claimPortion,
			UnlockHash: sfo.ClaimUnlockHash,
		}
		scoid := sfi.ParentID.SiaClaimOutputID()
		s.delayedSiacoinOutputs[s.height()][scoid] = sco
		bn.delayedSiacoinOutputs[scoid] = sco

		// Create the siafund output diff and remove the output from the
		// consensus set.
		bn.siafundOutputDiffs = append(bn.siafundOutputDiffs, modules.SiafundOutputDiff{
			Direction:     modules.DiffRevert,
			ID:            sfi.ParentID,
			SiafundOutput: s.siafundOutputs[sfi.ParentID],
		})
		delete(s.siafundOutputs, sfi.ParentID)
	}
}

// applySiafundOutputs takes all of the siafund outputs in a transaction and
// applies them to the state, updating the diffs in the block node.
func (s *State) applySiafundOutputs(bn *blockNode, t types.Transaction) {
	for i, sfo := range t.SiafundOutputs {
		// Sanity check - the output should not exist within the blockchain.
		sfoid := t.SiafundOutputID(i)
		if build.DEBUG {
			_, exists := s.siafundOutputs[sfoid]
			if exists {
				panic("siafund being added to consensus set when it is already in the consensus set")
			}
		}

		// Set the claim start.
		sfo.ClaimStart = s.siafundPool

		// Create and apply the diff.
		bn.siafundOutputDiffs = append(bn.siafundOutputDiffs, modules.SiafundOutputDiff{
			Direction:     modules.DiffApply,
			ID:            sfoid,
			SiafundOutput: sfo,
		})
		s.siafundOutputs[sfoid] = sfo
	}
}

// applyTransaction applies the contents of a transaction to the State. This
// produces a set of diffs, which are stored in the blockNode containing the
// transaction.
func (s *State) applyTransaction(bn *blockNode, t types.Transaction) {
	// Sanity check - the input transaction should be valid.
	if build.DEBUG {
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
