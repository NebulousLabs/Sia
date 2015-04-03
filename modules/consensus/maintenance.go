package consensus

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// applyMinerSubsidy adds a block's MinerPayouts to the State as delayed
// siacoin outputs. They are also recorded in the blockNode itself.
func (s *State) applyMinerSubsidy(bn *blockNode) {
	for i, payout := range bn.block.MinerPayouts {
		// Sanity check - the output should not already be in
		// delayedSiacoinOutputs, and should also not be in siacoinOutputs.
		id := bn.block.MinerPayoutID(i)
		if build.DEBUG {
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

// applyMaturedSiacoinOutputs goes through all of the outputs that
// have matured and adds them to the list of siacoinOutputs.
func (s *State) applyMaturedSiacoinOutputs(bn *blockNode) {
	for id, sco := range s.delayedSiacoinOutputs[bn.height-types.MaturityDelay] {
		// Sanity check - the output should not already be in siacoinOuptuts.
		if build.DEBUG {
			_, exists := s.siacoinOutputs[id]
			if exists {
				panic("trying to add a delayed output when the output is already there")
			}
		}

		// Add the output to the State and record the diff in the blockNode.
		s.siacoinOutputs[id] = sco
		bn.siacoinOutputDiffs = append(bn.siacoinOutputDiffs, modules.SiacoinOutputDiff{
			Direction:     modules.DiffApply,
			ID:            id,
			SiacoinOutput: sco,
		})
	}
}

// applyMissedProof adds the outputs and diffs that result from a contract
// expiring.
func (s *State) applyMissedProof(bn *blockNode, fcid types.FileContractID) {
	// Sanity check - the id must correspond to an existing contract.
	fc, exists := s.fileContracts[fcid]
	if !exists {
		if build.DEBUG {
			panic("misuse of applyMissedProof")
		}
		return
	}

	// Add all of the outputs in the missed proof outputs to the consensus set.
	for i, output := range fc.MissedProofOutputs {
		// Sanity check - output should not already exist.
		outputID := fcid.StorageProofOutputID(false, i)
		if build.DEBUG {
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

	// Remove the contract from the State and record the diff in the blockNode.
	delete(s.fileContracts, fcid)
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
	currentHeight := s.height()
	var expiredFileContracts []types.FileContractID
	for id, fc := range s.fileContracts {
		if fc.Expiration == currentHeight {
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
	s.applyMinerSubsidy(bn)
	s.applyMaturedSiacoinOutputs(bn)
	s.applyContractMaintenance(bn)
}
