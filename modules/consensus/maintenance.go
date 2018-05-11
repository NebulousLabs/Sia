package consensus

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus/database"
	"github.com/NebulousLabs/Sia/types"
)

var (
	errMissingFileContract = errors.New("storage proof submitted for non existing file contract")
	errOutputAlreadyMature = errors.New("delayed siacoin output is already in the matured outputs set")
	errPayoutsAlreadyPaid  = errors.New("payouts are already in the consensus set")
	errStorageProofTiming  = errors.New("missed proof triggered for file contract that is not expiring")
)

// applyMinerPayouts adds a block's miner payouts to the consensus set as
// delayed siacoin outputs.
func applyMinerPayouts(tx database.Tx, b *database.Block) {
	for i := range b.MinerPayouts {
		mpid := b.MinerPayoutID(uint64(i))
		dscod := modules.DelayedSiacoinOutputDiff{
			Direction:      modules.DiffApply,
			ID:             mpid,
			SiacoinOutput:  b.MinerPayouts[i],
			MaturityHeight: b.Height + types.MaturityDelay,
		}
		b.DelayedSiacoinOutputDiffs = append(b.DelayedSiacoinOutputDiffs, dscod)
		commitDelayedSiacoinOutputDiff(tx, dscod, modules.DiffApply)
	}
}

// applyMaturedSiacoinOutputs goes through the list of siacoin outputs that
// have matured and adds them to the consensus set. This also updates the block
// node diff set.
func applyMaturedSiacoinOutputs(tx database.Tx, b *database.Block) {
	// Skip this step if the blockchain is not old enough to have maturing
	// outputs.
	if b.Height < types.MaturityDelay {
		return
	}

	// Iterate through the list of delayed siacoin outputs.
	ids, scos := tx.DelayedSiacoinOutputs(b.Height)
	for i := range ids {
		id, sco := ids[i], scos[i]

		// Sanity check - the output should not already be in siacoinOuptuts.
		if build.DEBUG && isSiacoinOutput(tx, id) {
			panic(errOutputAlreadyMature)
		}

		// Add the output to the ConsensusSet and record the diff in the
		// blockNode.
		scod := modules.SiacoinOutputDiff{
			Direction:     modules.DiffApply,
			ID:            id,
			SiacoinOutput: sco,
		}
		b.SiacoinOutputDiffs = append(b.SiacoinOutputDiffs, scod)
		commitSiacoinOutputDiff(tx, scod, modules.DiffApply)

		// Add the delayed output to the ConsensusSet and record the diff in
		// the blockNode.
		dscod := modules.DelayedSiacoinOutputDiff{
			Direction:      modules.DiffRevert,
			ID:             id,
			SiacoinOutput:  sco,
			MaturityHeight: b.Height,
		}
		b.DelayedSiacoinOutputDiffs = append(b.DelayedSiacoinOutputDiffs, dscod)
		commitDelayedSiacoinOutputDiff(tx, dscod, modules.DiffApply)
	}
}

// applyMissedStorageProof adds the outputs and diffs that result from a file
// contract expiring.
func applyMissedStorageProof(tx database.Tx, b *database.Block, fcid types.FileContractID) (dscods []modules.DelayedSiacoinOutputDiff, fcd modules.FileContractDiff) {
	// Sanity checks.
	fc, err := getFileContract(tx, fcid)
	if build.DEBUG && err != nil {
		panic(err)
	}
	if build.DEBUG {
		// Check that the file contract in question expires at b.Height.
		if fc.WindowEnd != b.Height {
			panic(errStorageProofTiming)
		}
	}

	// Add all of the outputs in the missed proof outputs to the consensus set.
	for i, mpo := range fc.MissedProofOutputs {
		// Sanity check - output should not already exist.
		spoid := fcid.StorageProofOutputID(types.ProofMissed, uint64(i))
		if build.DEBUG && isSiacoinOutput(tx, spoid) {
			panic(errPayoutsAlreadyPaid)
		}

		// Don't add the output if the value is zero.
		dscod := modules.DelayedSiacoinOutputDiff{
			Direction:      modules.DiffApply,
			ID:             spoid,
			SiacoinOutput:  mpo,
			MaturityHeight: b.Height + types.MaturityDelay,
		}
		dscods = append(dscods, dscod)
	}

	// Remove the file contract from the consensus set and record the diff in
	// the blockNode.
	fcd = modules.FileContractDiff{
		Direction:    modules.DiffRevert,
		ID:           fcid,
		FileContract: fc,
	}
	return dscods, fcd
}

// applyFileContractMaintenance looks for all of the file contracts that have
// expired without an appropriate storage proof, and calls 'applyMissedProof'
// for the file contract.
func applyFileContractMaintenance(tx database.Tx, b *database.Block) {
	// Get all of the file contracts expiring at this height.
	var dscods []modules.DelayedSiacoinOutputDiff
	var fcds []modules.FileContractDiff
	for _, id := range tx.FileContractExpirations(b.Height) {
		amspDSCODS, fcd := applyMissedStorageProof(tx, b, id)
		fcds = append(fcds, fcd)
		dscods = append(dscods, amspDSCODS...)
	}
	for _, dscod := range dscods {
		b.DelayedSiacoinOutputDiffs = append(b.DelayedSiacoinOutputDiffs, dscod)
		commitDelayedSiacoinOutputDiff(tx, dscod, modules.DiffApply)
	}
	for _, fcd := range fcds {
		b.FileContractDiffs = append(b.FileContractDiffs, fcd)
		commitFileContractDiff(tx, fcd, modules.DiffApply)
	}
}

// applyMaintenance applies block-level alterations to the consensus set.
// Maintenance is applied after all of the transactions for the block have been
// applied.
func applyMaintenance(tx database.Tx, b *database.Block) {
	applyMinerPayouts(tx, b)
	applyMaturedSiacoinOutputs(tx, b)
	applyFileContractMaintenance(tx, b)
}
