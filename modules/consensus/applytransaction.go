package consensus

// applytransaction.go handles applying a transaction to the consensus set.
// There is an assumption that the transaction has already been verified.

import (
	"errors"

	"github.com/boltdb/bolt"

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
func (cs *ConsensusSet) applySiacoinInputs(tx *bolt.Tx, pb *processedBlock, t types.Transaction) error {
	// Remove all siacoin inputs from the unspent siacoin outputs list.
	for _, sci := range t.SiacoinInputs {
		sco, err := getSiacoinOutput(tx, sci.ParentID)
		if build.DEBUG && err != nil {
			panic(err)
		}
		scod := modules.SiacoinOutputDiff{
			Direction:     modules.DiffRevert,
			ID:            sci.ParentID,
			SiacoinOutput: sco,
		}
		pb.SiacoinOutputDiffs = append(pb.SiacoinOutputDiffs, scod)
		err = cs.commitTxSiacoinOutputDiff(tx, scod, modules.DiffApply)
		if err != nil {
			return err
		}
	}
	return nil
}

// applySiacoinOutputs takes all of the siacoin outputs in a transaction and
// applies them to the state, updating the diffs in the processed block.
func (cs *ConsensusSet) applySiacoinOutputs(tx *bolt.Tx, pb *processedBlock, t types.Transaction) error {
	// Add all siacoin outputs to the unspent siacoin outputs list.
	for i, sco := range t.SiacoinOutputs {
		scoid := t.SiacoinOutputID(i)
		scod := modules.SiacoinOutputDiff{
			Direction:     modules.DiffApply,
			ID:            scoid,
			SiacoinOutput: sco,
		}
		pb.SiacoinOutputDiffs = append(pb.SiacoinOutputDiffs, scod)
		err := cs.commitTxSiacoinOutputDiff(tx, scod, modules.DiffApply)
		if err != nil {
			return err
		}
	}
	return nil
}

// applyFileContracts iterates through all of the file contracts in a
// transaction and applies them to the state, updating the diffs in the proccesed
// block.
func (cs *ConsensusSet) applyFileContracts(tx *bolt.Tx, pb *processedBlock, t types.Transaction) error {
	for i, fc := range t.FileContracts {
		fcid := t.FileContractID(i)
		fcd := modules.FileContractDiff{
			Direction:    modules.DiffApply,
			ID:           fcid,
			FileContract: fc,
		}
		pb.FileContractDiffs = append(pb.FileContractDiffs, fcd)
		cs.commitTxFileContractDiff(tx, fcd, modules.DiffApply)

		// Get the portion of the contract that goes into the siafund pool and
		// add it to the siafund pool.
		sfp := getSiafundPool(tx)
		sfpd := modules.SiafundPoolDiff{
			Direction: modules.DiffApply,
			Previous:  sfp,
			Adjusted:  sfp.Add(fc.Tax()),
		}
		pb.SiafundPoolDiffs = append(pb.SiafundPoolDiffs, sfpd)
		cs.commitTxSiafundPoolDiff(tx, sfpd, modules.DiffApply)
	}
	return nil
}

// applyTxFileContractRevisions iterates through all of the file contract
// revisions in a transaction and applies them to the state, updating the diffs
// in the processed block.
func (cs *ConsensusSet) applyFileContractRevisions(tx *bolt.Tx, pb *processedBlock, t types.Transaction) error {
	for _, fcr := range t.FileContractRevisions {
		fc, err := getFileContract(tx, fcr.ParentID)
		if err != nil {
			return err
		}

		// Add the diff to delete the old file contract.
		fcd := modules.FileContractDiff{
			Direction:    modules.DiffRevert,
			ID:           fcr.ParentID,
			FileContract: fc,
		}
		pb.FileContractDiffs = append(pb.FileContractDiffs, fcd)
		err = cs.commitTxFileContractDiff(tx, fcd, modules.DiffApply)
		if err != nil {
			return err
		}

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
		err = cs.commitTxFileContractDiff(tx, fcd, modules.DiffApply)
		if err != nil {
			return err
		}
	}
	return nil
}

// applyTxStorageProofs iterates through all of the storage proofs in a
// transaction and applies them to the state, updating the diffs in the processed
// block.
func (cs *ConsensusSet) applyStorageProofs(tx *bolt.Tx, pb *processedBlock, t types.Transaction) error {
	for _, sp := range t.StorageProofs {
		fc, err := getFileContract(tx, sp.ParentID)
		if err != nil {
			return err
		}

		// Add all of the outputs in the ValidProofOutputs of the contract.
		for i, vpo := range fc.ValidProofOutputs {
			spoid := sp.ParentID.StorageProofOutputID(types.ProofValid, uint64(i))
			dscod := modules.DelayedSiacoinOutputDiff{
				Direction:      modules.DiffApply,
				ID:             spoid,
				SiacoinOutput:  vpo,
				MaturityHeight: pb.Height + types.MaturityDelay,
			}
			pb.DelayedSiacoinOutputDiffs = append(pb.DelayedSiacoinOutputDiffs, dscod)
			err := cs.commitTxDelayedSiacoinOutputDiff(tx, dscod, modules.DiffApply)
			if err != nil {
				return err
			}
		}

		fcd := modules.FileContractDiff{
			Direction:    modules.DiffRevert,
			ID:           sp.ParentID,
			FileContract: fc,
		}
		pb.FileContractDiffs = append(pb.FileContractDiffs, fcd)
		err = cs.commitTxFileContractDiff(tx, fcd, modules.DiffApply)
		if err != nil {
			return err
		}
	}
	return nil
}

// applyTxSiafundInputs takes all of the siafund inputs in a transaction and
// applies them to the state, updating the diffs in the processed block.
func (cs *ConsensusSet) applySiafundInputs(tx *bolt.Tx, pb *processedBlock, t types.Transaction) error {
	for _, sfi := range t.SiafundInputs {
		// Calculate the volume of siacoins to put in the claim output.
		sfo, err := getSiafundOutput(tx, sfi.ParentID)
		if err != nil {
			return err
		}
		claimPortion := getSiafundPool(tx).Sub(sfo.ClaimStart).Div(types.SiafundCount).Mul(sfo.Value)

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
		err = cs.commitTxDelayedSiacoinOutputDiff(tx, dscod, modules.DiffApply)
		if err != nil {
			return err
		}

		// Create the siafund output diff and remove the output from the
		// consensus set.
		sfod := modules.SiafundOutputDiff{
			Direction:     modules.DiffRevert,
			ID:            sfi.ParentID,
			SiafundOutput: sfo,
		}
		pb.SiafundOutputDiffs = append(pb.SiafundOutputDiffs, sfod)
		err = cs.commitTxSiafundOutputDiff(tx, sfod, modules.DiffApply)
		if err != nil {
			return err
		}
	}
	return nil
}

// applySiafundOutput applies a siafund output to the consensus set.
func (cs *ConsensusSet) applySiafundOutputs(tx *bolt.Tx, pb *processedBlock, t types.Transaction) error {
	for i, sfo := range t.SiafundOutputs {
		sfoid := t.SiafundOutputID(i)
		sfo.ClaimStart = getSiafundPool(tx)
		sfod := modules.SiafundOutputDiff{
			Direction:     modules.DiffApply,
			ID:            sfoid,
			SiafundOutput: sfo,
		}
		pb.SiafundOutputDiffs = append(pb.SiafundOutputDiffs, sfod)
		err := cs.commitTxSiafundOutputDiff(tx, sfod, modules.DiffApply)
		if err != nil {
			return err
		}
	}
	return nil
}

// applyTransaction applies the contents of a transaction to the ConsensusSet.
// This produces a set of diffs, which are stored in the blockNode containing
// the transaction. No verification is done by this function.
func (cs *ConsensusSet) applyTransaction(tx *bolt.Tx, pb *processedBlock, t types.Transaction) error {
	// Apply each component of the transaction. Miner fees are handled
	// elsewhere.
	err := cs.applySiacoinInputs(tx, pb, t)
	if err != nil {
		return err
	}
	err = cs.applySiacoinOutputs(tx, pb, t)
	if err != nil {
		return err
	}
	err = cs.applyFileContracts(tx, pb, t)
	if err != nil {
		return err
	}
	err = cs.applyFileContractRevisions(tx, pb, t)
	if err != nil {
		return err
	}
	err = cs.applyStorageProofs(tx, pb, t)
	if err != nil {
		return err
	}
	err = cs.applySiafundInputs(tx, pb, t)
	if err != nil {
		return err
	}
	err = cs.applySiafundOutputs(tx, pb, t)
	if err != nil {
		return err
	}
	return nil
}
