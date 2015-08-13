package consensus

import (
	"errors"

	"github.com/boltdb/bolt"

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
func (cs *ConsensusSet) applyMinerPayouts(pb *processedBlock) error {
	return cs.db.Update(func(tx *bolt.Tx) error {
		for i, payout := range pb.Block.MinerPayouts {
			// Create and apply the delayed miner payout.
			mpid := pb.Block.MinerPayoutID(uint64(i))
			dscod := modules.DelayedSiacoinOutputDiff{
				Direction:      modules.DiffApply,
				ID:             mpid,
				SiacoinOutput:  payout,
				MaturityHeight: pb.Height + types.MaturityDelay,
			}
			pb.DelayedSiacoinOutputDiffs = append(pb.DelayedSiacoinOutputDiffs, dscod)
			err := cs.commitTxDelayedSiacoinOutputDiff(tx, dscod, modules.DiffApply)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// applyMaturedSiacoinOutputs goes through the list of siacoin outputs that
// have matured and adds them to the consensus set. This also updates the block
// node diff set.
func (cs *ConsensusSet) applyMaturedSiacoinOutputs(pb *processedBlock) error {
	// Skip this step if the blockchain is not old enough to have maturing
	// outputs.
	if !(pb.Height > types.MaturityDelay) {
		return nil
	}

	// Gather the matured outputs from the delayed outputs map
	var dscoids []types.SiacoinOutputID
	var dscos []types.SiacoinOutput
	cs.db.forEachDelayedSiacoinOutputsHeight(pb.Height, func(id types.SiacoinOutputID, sco types.SiacoinOutput) {
		dscoids = append(dscoids, id)
		dscos = append(dscos, sco)
	})
	// Add all of the matured outputs to the full siaocin output set.
	for i := 0; i < len(dscos); i++ {
		dscoid := dscoids[i]
		dsco := dscos[i]

		// Sanity check - the output should not already be in siacoinOuptuts.
		if build.DEBUG {
			exists := cs.db.inSiacoinOutputs(dscoid)
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
		err := cs.db.Update(func(tx *bolt.Tx) error {
			return cs.commitTxSiacoinOutputDiff(tx, scod, modules.DiffApply)
		})
		if err != nil {
			return err
		}

		// Remove the delayed siacoin output from the consensus set.
		dscod := modules.DelayedSiacoinOutputDiff{
			Direction:      modules.DiffRevert,
			ID:             dscoid,
			SiacoinOutput:  dsco,
			MaturityHeight: pb.Height,
		}
		pb.DelayedSiacoinOutputDiffs = append(pb.DelayedSiacoinOutputDiffs, dscod)
		err = cs.db.Update(func(tx *bolt.Tx) error {
			return cs.commitTxDelayedSiacoinOutputDiff(tx, dscod, modules.DiffApply)
		})
		if err != nil {
			return err
		}
	}

	// Delete the map that held the now-matured outputs.
	// Sanity check - map should be empty.
	if build.DEBUG {
		if cs.db.lenDelayedSiacoinOutputsHeight(pb.Height) != 0 {
			panic("deleting non-empty map")
		}
	}
	cs.db.rmDelayedSiacoinOutputs(pb.Height)
	return nil
}

// applyMissedStorageProof adds the outputs and diffs that result from a file
// contract expiring.
func (cs *ConsensusSet) applyMissedStorageProof(pb *processedBlock, fcid types.FileContractID) error {
	// Sanity checks.
	fc := cs.db.getFileContracts(fcid)
	if build.DEBUG {
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
			exists := cs.db.inDelayedSiacoinOutputsHeight(pb.Height+types.MaturityDelay, spoid)
			if exists {
				panic(errPayoutsAlreadyPaid)
			}
			exists = cs.db.inSiacoinOutputs(spoid)
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
		err := cs.db.Update(func(tx *bolt.Tx) error {
			return cs.commitTxDelayedSiacoinOutputDiff(tx, dscod, modules.DiffApply)
		})
		if err != nil {
			return err
		}
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

	return nil
}

// applyFileContractMaintenance looks for all of the file contracts that have
// expired without an appropriate storage proof, and calls 'applyMissedProof'
// for the file contract.
func (cs *ConsensusSet) applyFileContractMaintenance(pb *processedBlock) error {
	// Skip if there are no expirations at this height.
	if !cs.db.inFCExpirations(pb.Height) {
		return nil
	}

	// Grab all of the expired file contracts.
	var expiredFileContracts []types.FileContractID
	cs.db.forEachFCExpirationsHeight(pb.Height, func(id types.FileContractID) {
		expiredFileContracts = append(expiredFileContracts, id)
	})
	for _, id := range expiredFileContracts {
		err := cs.applyMissedStorageProof(pb, id)
		if err != nil {
			return err
		}
	}
	cs.db.rmFCExpirations(pb.Height)
	return nil
}

// applyMaintenance applies block-level alterations to the consensus set.
// Maintenance is applied after all of the transcations for the block have been
// applied.
func (cs *ConsensusSet) applyMaintenance(pb *processedBlock) error {
	err := cs.applyMinerPayouts(pb)
	if err != nil {
		return err
	}
	err = cs.applyMaturedSiacoinOutputs(pb)
	if err != nil {
		return err
	}
	err = cs.applyFileContractMaintenance(pb)
	if err != nil {
		return err
	}
	return nil
}
