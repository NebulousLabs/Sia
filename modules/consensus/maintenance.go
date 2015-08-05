package consensus

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
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
func (cs *ConsensusSet) applyMinerPayouts(pb *processedBlock) {
	for i, payout := range pb.Block.MinerPayouts {
		// Sanity check - input should not exist in the consensus set.
		mpid := pb.Block.MinerPayoutID(uint64(i))
		if build.DEBUG {
			// Check the delayed outputs set.
			_, exists := cs.delayedSiacoinOutputs[pb.Height+types.MaturityDelay][mpid]
			if exists {
				panic(errPayoutsAlreadyPaid)
			}
			// Check the full outputs set.
			_, exists = cs.siacoinOutputs[mpid]
			if exists {
				panic(errPayoutsAlreadyPaid)
			}
		}

		// Create and apply the delayed miner payout.
		dscod := modules.DelayedSiacoinOutputDiff{
			Direction:      modules.DiffApply,
			ID:             mpid,
			SiacoinOutput:  payout,
			MaturityHeight: pb.Height + types.MaturityDelay,
		}
		pb.DelayedSiacoinOutputDiffs = append(pb.DelayedSiacoinOutputDiffs, dscod)
		cs.commitDelayedSiacoinOutputDiff(dscod, modules.DiffApply)
	}
	return
}

// applyMaturedSiacoinOutputs goes through the list of siacoin outputs that
// have matured and adds them to the consensus set. This also updates the block
// node diff set.
func (cs *ConsensusSet) applyMaturedSiacoinOutputs(pb *processedBlock) {
	// Skip this step if the blockchain is not old enough to have maturing
	// outputs.
	if !(pb.Height > types.MaturityDelay) {
		return
	}

	// Add all of the matured outputs to the full siaocin output set.
	for dscoid, dsco := range cs.delayedSiacoinOutputs[pb.Height] {
		// Sanity check - the output should not already be in siacoinOuptuts.
		if build.DEBUG {
			_, exists := cs.siacoinOutputs[dscoid]
			if exists {
				panic(errOutputAlreadyMature)
			}
		}

		// Add the output to the ConsensusSet and record the diff in the
		// blockNode.
		scod := modules.SiacoinOutputDiff{
			Direction:     modules.DiffApply,
			ID:            dscoid,
			SiacoinOutput: dsco,
		}
		pb.SiacoinOutputDiffs = append(pb.SiacoinOutputDiffs, scod)
		cs.commitSiacoinOutputDiff(scod, modules.DiffApply)

		// Remove the delayed siacoin output from the consensus set.
		dscod := modules.DelayedSiacoinOutputDiff{
			Direction:      modules.DiffRevert,
			ID:             dscoid,
			SiacoinOutput:  dsco,
			MaturityHeight: pb.Height,
		}
		pb.DelayedSiacoinOutputDiffs = append(pb.DelayedSiacoinOutputDiffs, dscod)
		cs.commitDelayedSiacoinOutputDiff(dscod, modules.DiffApply)
	}

	// Delete the map that held the now-matured outputs.
	// Sanity check - map should be empty.
	if build.DEBUG {
		if len(cs.delayedSiacoinOutputs[pb.Height]) != 0 {
			panic("deleting non-empty map")
		}
	}
	delete(cs.delayedSiacoinOutputs, pb.Height)
}

// applyMissedStorageProof adds the outputs and diffs that result from a file
// contract expiring.
func (cs *ConsensusSet) applyMissedStorageProof(pb *processedBlock, fcid types.FileContractID) {
	// Sanity checks.
	fc, exists := cs.fileContracts[fcid]
	if build.DEBUG {
		// Check that the file contract in question exists.
		if !exists {
			panic(errMissingFileContract)
		}

		// Check that the file contract in question expires at pb.Height.
		if fc.WindowEnd != pb.Height {
			panic(errStorageProofTiming)
		}
	}

	// Add all of the outputs in the missed proof outputs to the consensus set.
	for i, mpo := range fc.MissedProofOutputs {
		// Sanity check - output should not already exist.
		spoid := fcid.StorageProofOutputID(types.ProofMissed, uint64(i))
		if build.DEBUG {
			_, exists := cs.delayedSiacoinOutputs[pb.Height+types.MaturityDelay][spoid]
			if exists {
				panic(errPayoutsAlreadyPaid)
			}
			_, exists = cs.siacoinOutputs[spoid]
			if exists {
				panic(errPayoutsAlreadyPaid)
			}
		}

		dscod := modules.DelayedSiacoinOutputDiff{
			Direction:      modules.DiffApply,
			ID:             spoid,
			SiacoinOutput:  mpo,
			MaturityHeight: pb.Height + types.MaturityDelay,
		}
		pb.DelayedSiacoinOutputDiffs = append(pb.DelayedSiacoinOutputDiffs, dscod)
		cs.commitDelayedSiacoinOutputDiff(dscod, modules.DiffApply)
	}

	// Remove the file contract from the consensus set and record the diff in
	// the blockNode.
	fcd := modules.FileContractDiff{
		Direction:    modules.DiffRevert,
		ID:           fcid,
		FileContract: fc,
	}
	pb.FileContractDiffs = append(pb.FileContractDiffs, fcd)
	cs.commitFileContractDiff(fcd, modules.DiffApply)

	return
}

// applyFileContractMaintenance looks for all of the file contracts that have
// expired without an appropriate storage proof, and calls 'applyMissedProof'
// for the file contract.
func (cs *ConsensusSet) applyFileContractMaintenance(pb *processedBlock) {
	// Because you can't modify a map safely while iterating through it, a
	// slice of contracts to be handled is created, then acted upon after
	// iterating through the map.
	var expiredFileContracts []types.FileContractID
	for id, _ := range cs.fileContractExpirations[pb.Height] {
		expiredFileContracts = append(expiredFileContracts, id)
	}
	for _, id := range expiredFileContracts {
		cs.applyMissedStorageProof(pb, id)
	}

	// Sanity check - map with expiring file contracts should now be empty.
	if build.DEBUG {
		if len(cs.fileContractExpirations[pb.Height]) != 0 {
			panic("an expiring file contract was missed")
		}
	}
	delete(cs.fileContractExpirations, pb.Height)

	return
}

// applyMaintenance applies block-level alterations to the consensus set.
// Maintenance is applied after all of the transcations for the block have been
// applied.
func (cs *ConsensusSet) applyMaintenance(pb *processedBlock) {
	cs.applyMinerPayouts(pb)
	cs.applyMaturedSiacoinOutputs(pb)
	cs.applyFileContractMaintenance(pb)
}
