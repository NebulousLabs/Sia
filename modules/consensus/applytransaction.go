package consensus

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	ErrDuplicateValidProofOutput        = errors.New("applying a storage proof created a duplicate proof output")
	ErrMisuseApplySiacoinInput          = errors.New("applying a transaction with an invalid unspent siacoin output")
	ErrMisuseApplySiacoinOutput         = errors.New("applying a transaction with an invalid siacoin output")
	ErrMisuseApplyFileContracts         = errors.New("applying a transaction with an invalid file contract")
	ErrMisuseApplyFileContractRevisions = errors.New("applying a revision for a nonexistant file contract")
	ErrMisuseApplySiafundInput          = errors.New("applying a transaction with invalid siafund input")
	ErrMisuseApplySiafundOutput         = errors.New("applying a transaction with an invalid siafund output")
	ErrNonexistentStorageProof          = errors.New("applying a storage proof for a nonexistent file contract")
)

// applySiacoinInputs takes all of the siacoin inputs in a transaction and
// applies them to the state, updating the diffs in the processed block.
func (cs *ConsensusSet) applySiacoinInputs(pb *processedBlock, t types.Transaction) {
	// Remove all siacoin inputs from the unspent siacoin outputs list.
	for _, sci := range t.SiacoinInputs {
		// Sanity check - the input should exist within the blockchain.
		if build.DEBUG {
			_, exists := cs.siacoinOutputs[sci.ParentID]
			if !exists {
				panic(ErrMisuseApplySiacoinInput)
			}
		}

		scod := modules.SiacoinOutputDiff{
			Direction:     modules.DiffRevert,
			ID:            sci.ParentID,
			SiacoinOutput: cs.siacoinOutputs[sci.ParentID],
		}
		pb.SiacoinOutputDiffs = append(pb.SiacoinOutputDiffs, scod)
		cs.commitSiacoinOutputDiff(scod, modules.DiffApply)
	}
}

// applySiacoinOutputs takes all of the siacoin outputs in a transaction and
// applies them to the state, updating the diffs in the processed block.
func (cs *ConsensusSet) applySiacoinOutputs(pb *processedBlock, t types.Transaction) {
	// Add all siacoin outputs to the unspent siacoin outputs list.
	for i, sco := range t.SiacoinOutputs {
		// Sanity check - the output should not exist within the state.
		scoid := t.SiacoinOutputID(i)
		if build.DEBUG {
			_, exists := cs.siacoinOutputs[scoid]
			if exists {
				panic(ErrMisuseApplySiacoinOutput)
			}
		}

		scod := modules.SiacoinOutputDiff{
			Direction:     modules.DiffApply,
			ID:            scoid,
			SiacoinOutput: sco,
		}
		pb.SiacoinOutputDiffs = append(pb.SiacoinOutputDiffs, scod)
		cs.commitSiacoinOutputDiff(scod, modules.DiffApply)
	}
}

// applyFileContracts iterates through all of the file contracts in a
// transaction and applies them to the state, updating the diffs in the proccesed
// block.
func (cs *ConsensusSet) applyFileContracts(pb *processedBlock, t types.Transaction) {
	for i, fc := range t.FileContracts {
		// Sanity check - the file contract should not exists within the state.
		fcid := t.FileContractID(i)
		if build.DEBUG {
			_, exists := cs.fileContracts[fcid]
			if exists {
				panic(ErrMisuseApplyFileContracts)
			}
		}

		fcd := modules.FileContractDiff{
			Direction:    modules.DiffApply,
			ID:           fcid,
			FileContract: fc,
		}
		pb.FileContractDiffs = append(pb.FileContractDiffs, fcd)
		cs.commitFileContractDiff(fcd, modules.DiffApply)

		// Get the portion of the contract that goes into the siafund pool and
		// add it to the siafund pool.
		sfpd := modules.SiafundPoolDiff{
			Direction: modules.DiffApply,
			Previous:  cs.siafundPool,
			Adjusted:  cs.siafundPool.Add(fc.Tax()),
		}
		pb.SiafundPoolDiffs = append(pb.SiafundPoolDiffs, sfpd)
		cs.commitSiafundPoolDiff(sfpd, modules.DiffApply)
	}
	return
}

// applyFileContractRevisions iterates through all of the file contract
// revisions in a transaction and applies them to the state, updating the diffs
// in the processed block.
func (cs *ConsensusSet) applyFileContractRevisions(pb *processedBlock, t types.Transaction) {
	for _, fcr := range t.FileContractRevisions {
		// Sanity check - termination should affect an existing contract.
		fc, exists := cs.fileContracts[fcr.ParentID]
		if build.DEBUG {
			if !exists {
				panic(ErrMisuseApplyFileContractRevisions)
			}
		}

		// Add the diff to delete the old file contract.
		fcd := modules.FileContractDiff{
			Direction:    modules.DiffRevert,
			ID:           fcr.ParentID,
			FileContract: fc,
		}
		pb.FileContractDiffs = append(pb.FileContractDiffs, fcd)
		cs.commitFileContractDiff(fcd, modules.DiffApply)

		// Add the diff to add the revised file contract.
		newFC := types.FileContract{
			FileSize:           fcr.NewFileSize,
			FileMerkleRoot:     fcr.NewFileMerkleRoot,
			WindowStart:        fcr.NewWindowStart,
			WindowEnd:          fcr.NewWindowEnd,
			Payout:             fc.Payout,
			ValidProofOutputs:  fcr.NewValidProofOutputs,
			MissedProofOutputs: fcr.NewMissedProofOutputs,
			UnlockHash:         fcr.NewUnlockHash,
			RevisionNumber:     fcr.NewRevisionNumber,
		}
		fcd = modules.FileContractDiff{
			Direction:    modules.DiffApply,
			ID:           fcr.ParentID,
			FileContract: newFC,
		}
		pb.FileContractDiffs = append(pb.FileContractDiffs, fcd)
		cs.commitFileContractDiff(fcd, modules.DiffApply)
	}
}

// applyStorageProofs iterates through all of the storage proofs in a
// transaction and applies them to the state, updating the diffs in the processed
// block.
func (cs *ConsensusSet) applyStorageProofs(pb *processedBlock, t types.Transaction) {
	for _, sp := range t.StorageProofs {
		// Sanity check - the file contract of the storage proof should exist.
		fc, exists := cs.fileContracts[sp.ParentID]
		if build.DEBUG {
			if !exists {
				panic(ErrNonexistentStorageProof)
			}
		}

		// Add all of the outputs in the ValidProofOutputs of the contract.
		for i, vpo := range fc.ValidProofOutputs {
			// Sanity check - output should not already exist.
			spoid := sp.ParentID.StorageProofOutputID(types.ProofValid, uint64(i))
			if build.DEBUG {
				_, exists := cs.delayedSiacoinOutputs[pb.Height+types.MaturityDelay][spoid]
				if exists {
					panic(ErrDuplicateValidProofOutput)
				}
			}

			dscod := modules.DelayedSiacoinOutputDiff{
				Direction:      modules.DiffApply,
				ID:             spoid,
				SiacoinOutput:  vpo,
				MaturityHeight: pb.Height + types.MaturityDelay,
			}
			pb.DelayedSiacoinOutputDiffs = append(pb.DelayedSiacoinOutputDiffs, dscod)
			cs.commitDelayedSiacoinOutputDiff(dscod, modules.DiffApply)
		}

		fcd := modules.FileContractDiff{
			Direction:    modules.DiffRevert,
			ID:           sp.ParentID,
			FileContract: fc,
		}
		pb.FileContractDiffs = append(pb.FileContractDiffs, fcd)
		cs.commitFileContractDiff(fcd, modules.DiffApply)
	}
	return
}

// applySiafundInputs takes all of the siafund inputs in a transaction and
// applies them to the state, updating the diffs in the processed block.
func (cs *ConsensusSet) applySiafundInputs(pb *processedBlock, t types.Transaction) {
	for _, sfi := range t.SiafundInputs {
		// Sanity check - the input should exist within the blockchain.
		if build.DEBUG {
			_, exists := cs.siafundOutputs[sfi.ParentID]
			if !exists {
				panic(ErrMisuseApplySiafundInput)
			}
		}

		// Calculate the volume of siacoins to put in the claim output.
		sfo := cs.siafundOutputs[sfi.ParentID]
		claimPortion := cs.siafundPool.Sub(sfo.ClaimStart).Div(types.SiafundCount).Mul(sfo.Value)

		// Add the claim output to the delayed set of outputs.
		sco := types.SiacoinOutput{
			Value:      claimPortion,
			UnlockHash: sfi.ClaimUnlockHash,
		}
		scoid := sfi.ParentID.SiaClaimOutputID()
		dscod := modules.DelayedSiacoinOutputDiff{
			Direction:      modules.DiffApply,
			ID:             scoid,
			SiacoinOutput:  sco,
			MaturityHeight: pb.Height + types.MaturityDelay,
		}
		pb.DelayedSiacoinOutputDiffs = append(pb.DelayedSiacoinOutputDiffs, dscod)
		cs.commitDelayedSiacoinOutputDiff(dscod, modules.DiffApply)

		// Create the siafund output diff and remove the output from the
		// consensus set.
		sfod := modules.SiafundOutputDiff{
			Direction:     modules.DiffRevert,
			ID:            sfi.ParentID,
			SiafundOutput: cs.siafundOutputs[sfi.ParentID],
		}
		pb.SiafundOutputDiffs = append(pb.SiafundOutputDiffs, sfod)
		cs.commitSiafundOutputDiff(sfod, modules.DiffApply)
	}
}

// applySiafundOutputs takes all of the siafund outputs in a transaction and
// applies them to the state, updating the diffs in the processed block.
func (cs *ConsensusSet) applySiafundOutputs(pb *processedBlock, t types.Transaction) {
	for i, sfo := range t.SiafundOutputs {
		// Sanity check - the output should not exist within the blockchain.
		sfoid := t.SiafundOutputID(i)
		if build.DEBUG {
			_, exists := cs.siafundOutputs[sfoid]
			if exists {
				panic(ErrMisuseApplySiafundOutput)
			}
		}

		// Set the claim start.
		sfo.ClaimStart = cs.siafundPool

		// Create and apply the diff.
		sfod := modules.SiafundOutputDiff{
			Direction:     modules.DiffApply,
			ID:            sfoid,
			SiafundOutput: sfo,
		}
		pb.SiafundOutputDiffs = append(pb.SiafundOutputDiffs, sfod)
		cs.commitSiafundOutputDiff(sfod, modules.DiffApply)
	}
}

// applyTransaction applies the contents of a transaction to the ConsensusSet.
// This produces a set of diffs, which are stored in the blockNode containing
// the transaction. No verification is done by this function.
func (cs *ConsensusSet) applyTransaction(pb *processedBlock, t types.Transaction) {
	// Apply each component of the transaction. Miner fees are handled
	// elsewhere.
	cs.applySiacoinInputs(pb, t)
	cs.applySiacoinOutputs(pb, t)
	cs.applyFileContracts(pb, t)
	cs.applyFileContractRevisions(pb, t)
	cs.applyStorageProofs(pb, t)
	cs.applySiafundInputs(pb, t)
	cs.applySiafundOutputs(pb, t)
}
