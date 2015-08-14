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
func (cs *ConsensusSet) applyMinerPayouts(tx *bolt.Tx, pb *processedBlock) error {
	for i := range pb.Block.MinerPayouts {
		// Create and apply the delayed miner payout.
		mpid := pb.Block.MinerPayoutID(uint64(i))
		dscod := modules.DelayedSiacoinOutputDiff{
			Direction:      modules.DiffApply,
			ID:             mpid,
			SiacoinOutput:  pb.Block.MinerPayouts[i],
			MaturityHeight: pb.Height + types.MaturityDelay,
		}
		pb.DelayedSiacoinOutputDiffs = append(pb.DelayedSiacoinOutputDiffs, dscod)
		err := cs.commitTxDelayedSiacoinOutputDiff(tx, dscod, modules.DiffApply)
		if err != nil {
			return err
		}
	}
	return nil
}

// applyMaturedSiacoinOutputs goes through the list of siacoin outputs that
// have matured and adds them to the consensus set. This also updates the block
// node diff set.
func (cs *ConsensusSet) applyMaturedSiacoinOutputs(tx *bolt.Tx, pb *processedBlock) error {
	// Skip this step if the blockchain is not old enough to have maturing
	// outputs.
	if !(pb.Height > types.MaturityDelay) {
		return nil
	}

	scoBucket := tx.Bucket(SiacoinOutputs)
	err := forEachDSCO(tx, pb.Height, func(id types.SiacoinOutputID, sco types.SiacoinOutput) error {
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
		pb.SiacoinOutputDiffs = append(pb.SiacoinOutputDiffs, scod)
		err := cs.commitBucketSiacoinOutputDiff(scoBucket, scod, modules.DiffApply)
		if err != nil {
			return err
		}

		// Remove the delayed siacoin output from the consensus set.
		dscod := modules.DelayedSiacoinOutputDiff{
			Direction:      modules.DiffRevert,
			ID:             id,
			SiacoinOutput:  sco,
			MaturityHeight: pb.Height,
		}
		pb.DelayedSiacoinOutputDiffs = append(pb.DelayedSiacoinOutputDiffs, dscod)
		return cs.commitTxDelayedSiacoinOutputDiff(tx, dscod, modules.DiffApply)
	})
	if err != nil {
		return err
	}
	return removeDSCOBucket(tx, pb.Height)
}

// applyMissedStorageProof adds the outputs and diffs that result from a file
// contract expiring.
func (cs *ConsensusSet) applyTxMissedStorageProof(tx *bolt.Tx, pb *processedBlock, fcid types.FileContractID) error {
	// Sanity checks.
	fc, err := getFileContract(tx, fcid)
	if err != nil {
		return err
	}
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
			exists := isSiacoinOutput(tx, spoid)
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
		err = cs.commitTxDelayedSiacoinOutputDiff(tx, dscod, modules.DiffApply)
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
	return cs.commitTxFileContractDiff(tx, fcd, modules.DiffApply)
}

// applyFileContractMaintenance looks for all of the file contracts that have
// expired without an appropriate storage proof, and calls 'applyMissedProof'
// for the file contract.
func (cs *ConsensusSet) applyFileContractMaintenance(pb *processedBlock) error {
	// Skip if there are no expirations at this height.
	if !cs.db.inFCExpirations(pb.Height) {
		return nil
	}

	err := cs.db.Update(func(tx *bolt.Tx) error {
		return forEachFCExpiration(tx, pb.Height, func(id types.FileContractID) error {
			return cs.applyTxMissedStorageProof(tx, pb, id)
		})
	})
	if err != nil {
		return err
	}
	cs.db.rmFCExpirations(pb.Height)
	return nil
}

// applyMaintenance applies block-level alterations to the consensus set.
// Maintenance is applied after all of the transcations for the block have been
// applied.
func (cs *ConsensusSet) applyMaintenance(pb *processedBlock) error {
	err := cs.db.Update(func(tx *bolt.Tx) error {
		err := cs.applyMinerPayouts(tx, pb)
		if err != nil {
			return err
		}
		return cs.applyMaturedSiacoinOutputs(tx, pb)
	})
	if err != nil {
		return err
	}
	return cs.applyFileContractMaintenance(pb)
}
