package consensus

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// applyMinerPayouts adds a block's miner payouts to the consensus set as
// delayed siacoin outputs.
func (cs *State) applyMinerPayouts(bn *blockNode) {
	for i, payout := range bn.block.MinerPayouts {
		// Sanity check - input should not exist in the consensus set.
		mpid := bn.block.MinerPayoutID(i)
		if build.DEBUG {
			// Check the delayed outputs set.
			_, exists := cs.delayedSiacoinOutputs[bn.height+types.MaturityDelay][mpid]
			if exists {
				panic("miner subsidy already in delayed outputs")
			}
			// Check the full outputs set.
			_, exists = cs.siacoinOutputs[mpid]
			if exists {
				panic("miner subsidy already in siacoin outputs")
			}
		}

		// Create and apply the delayed miner payout.
		dscod := modules.DelayedSiacoinOutputDiff{
			Direction:      modules.DiffApply,
			ID:             mpid,
			SiacoinOutput:  payout,
			MaturityHeight: bn.height + types.MaturityDelay,
		}
		bn.delayedSiacoinOutputDiffs = append(bn.delayedSiacoinOutputDiffs, dscod)
		cs.commitDelayedSiacoinOutputDiff(dscod, modules.DiffApply)
	}
	return
}

// applyMaturedSiacoinOutputs goes through the list of siacoin outputs that
// have matured and adds them to the consensus set. This also updates the block
// node diff set.
func (cs *State) applyMaturedSiacoinOutputs(bn *blockNode) {
	// Skip this step if the blockchain is not old enough to have maturing
	// outputs.
	if !(bn.height > types.MaturityDelay) {
		return
	}

	// Add all of the matured outputs to the full siaocin output set.
	for dscoid, dsco := range cs.delayedSiacoinOutputs[bn.height] {
		// Sanity check - the output should not already be in siacoinOuptuts.
		if build.DEBUG {
			_, exists := cs.siacoinOutputs[dscoid]
			if exists {
				panic("trying to add a delayed output when the output is already there")
			}
		}

		// Add the output to the State and record the diff in the blockNode.
		scod := modules.SiacoinOutputDiff{
			Direction:     modules.DiffApply,
			ID:            dscoid,
			SiacoinOutput: dsco,
		}
		bn.siacoinOutputDiffs = append(bn.siacoinOutputDiffs, scod)
		cs.commitSiacoinOutputDiff(scod, modules.DiffApply)

		// Remove the delayed siacoin output from the consensus set.
		dscod := modules.DelayedSiacoinOutputDiff{
			Direction:      modules.DiffRevert,
			ID:             dscoid,
			SiacoinOutput:  dsco,
			MaturityHeight: bn.height,
		}
		bn.delayedSiacoinOutputDiffs = append(bn.delayedSiacoinOutputDiffs, dscod)
		cs.commitDelayedSiacoinOutputDiff(dscod, modules.DiffApply)
	}

	// Delete the map that held the now-matured outputs.
	// Sanity check - map should be empty.
	if build.DEBUG {
		if len(cs.delayedSiacoinOutputs[bn.height]) != 0 {
			panic("deleting non-empty map")
		}
	}
	delete(cs.delayedSiacoinOutputs, bn.height)
}

// applyMissedStorageProof adds the outputs and diffs that result from a file
// contract expiring.
func (cs *State) applyMissedStorageProof(bn *blockNode, fcid types.FileContractID) {
	// Sanity checks.
	fc, exists := cs.fileContracts[fcid]
	if build.DEBUG {
		// Check that the file contract in question exists.
		if !exists {
			panic("misuse of applyMissedProof")
		}

		// Check that the file contract in question expires at bn.height.
		if fc.WindowEnd != bn.height {
			panic("applyMissedStorageProof being called at the wrong height")
		}
	}

	// Add the siafund tax to the siafund pool.
	cs.siafundPool = cs.siafundPool.Add(fc.Tax())

	// Add all of the outputs in the missed proof outputs to the consensus set.
	for i, mpo := range fc.MissedProofOutputs {
		// Sanity check - output should not already exist.
		spid := fcid.StorageProofOutputID(false, i)
		if build.DEBUG {
			_, exists := cs.delayedSiacoinOutputs[bn.height+types.MaturityDelay][spid]
			if exists {
				panic("missed proof output already exists in the delayed outputs set")
			}
			_, exists = cs.siacoinOutputs[spid]
			if exists {
				panic("missed proof output already exists in the siacoin outputs set")
			}
		}

		dscod := modules.DelayedSiacoinOutputDiff{
			Direction:      modules.DiffApply,
			ID:             spid,
			SiacoinOutput:  mpo,
			MaturityHeight: bn.height + types.MaturityDelay,
		}
		bn.delayedSiacoinOutputDiffs = append(bn.delayedSiacoinOutputDiffs, dscod)
		cs.commitDelayedSiacoinOutputDiff(dscod, modules.DiffApply)
	}

	// Remove the contract from the State and record the diff in the blockNode.
	delete(cs.fileContracts, fcid)
	bn.fileContractDiffs = append(bn.fileContractDiffs, modules.FileContractDiff{
		Direction:    modules.DiffRevert,
		ID:           fcid,
		FileContract: fc,
	})

	return
}

// applyFileContractMaintenance looks for all of the file contracts that have
// expired without an appropriate storage proof, and calls 'applyMissedProof'
// for the file contract.
func (cs *State) applyFileContractMaintenance(bn *blockNode) {
	// Because you can't modify a map safely while iterating through it, a
	// slice of contracts to be handled is created, then acted upon after
	// iterating through the map.
	var expiredFileContracts []types.FileContractID
	for id, fc := range cs.fileContracts {
		if fc.WindowEnd == bn.height {
			expiredFileContracts = append(expiredFileContracts, id)
		}

		// Sanity check - there should be no file contracts in the consensus
		// set at a lower height than the block node height.
		if build.DEBUG {
			if fc.WindowEnd < bn.height {
				panic("an expiring file contract was missed somehow")
			}
		}
	}
	for _, id := range expiredFileContracts {
		cs.applyMissedStorageProof(bn, id)
	}

	return
}

// applyMaintenance applies block-level alterations to the consensus set.
// Maintenance is applied after all of the transcations for the block have been
// applied.
func (cs *State) applyMaintenance(bn *blockNode) {
	cs.applyMinerPayouts(bn)
	cs.applyMaturedSiacoinOutputs(bn)
	cs.applyFileContractMaintenance(bn)
}
