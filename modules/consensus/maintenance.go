package consensus

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// applyMinerPayouts adds a block's MinerPayouts to the ConsensusSet as delayed
// siacoin outputs. They are also recorded in the blockNode itself.
func (cs *State) applyMinerPayouts(bn *blockNode) {
	for i, payout := range bn.block.MinerPayouts {
		// Sanity check - the output should not already be in
		// delayedSiacoinOutputs, and should also not be in siacoinOutputs.
		mpid := bn.block.MinerPayoutID(i)
		if build.DEBUG {
			_, exists := cs.delayedSiacoinOutputs[bn.height+types.MaturityDelay][mpid]
			if exists {
				panic("miner subsidy already in delayed outputs")
			}
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

// applyMaturedSiacoinOutputs goes through all of the outputs that
// have matured and adds them to the list of siacoinOutputs.
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

// applyMissedProof adds the outputs and diffs that result from a contract
// expiring.
func (cs *State) applyMissedProof(bn *blockNode, fcid types.FileContractID) {
	// Sanity check - the id must correspond to an existing contract.
	fc, exists := cs.fileContracts[fcid]
	if !exists {
		if build.DEBUG {
			panic("misuse of applyMissedProof")
		}
		return
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

// applyContractMaintenance iterates through all of the contracts in the
// consensus set and calls 'applyMissedProof' on any that have expired.
func (s *State) applyContractMaintenance(bn *blockNode) {
	// Iterate through all contracts and figure out which ones have expired.
	// Expiring a contract deletes it from the map we are iterating through, so
	// we need to store it and deleted once we're done iterating through the
	// map.
	currentHeight := bn.height
	var expiredFileContracts []types.FileContractID
	for id, fc := range s.fileContracts {
		if fc.WindowEnd == currentHeight {
			expiredFileContracts = append(expiredFileContracts, id)
		}
	}
	for _, id := range expiredFileContracts {
		s.applyMissedProof(bn, id)
	}

	return
}

// applyMaintenance generates, adds, and applies diffs that are generated after
// all of the transactions of a block have been processed. This includes adding
// the miner susidies, adding any matured outputs to the set of siacoin
// outputs, and dealing with any contracts that have expired.
func (s *State) applyMaintenance(bn *blockNode) {
	s.applyMinerPayouts(bn)
	s.applyMaturedSiacoinOutputs(bn)
	s.applyContractMaintenance(bn)
}
