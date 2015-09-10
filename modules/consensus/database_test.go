package consensus

// database_test.go contains a bunch of legacy functions to preserve
// compatibility with the test suite.

import (
	"github.com/boltdb/bolt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// dbBlockHeight is a convenience function allowing blockHeight to be called
// without a bolt.Tx.
func (cs *ConsensusSet) dbBlockHeight() (bh types.BlockHeight) {
	_ = cs.db.Update(func(tx *bolt.Tx) error {
		bh = blockHeight(tx)
		return nil
	})
	return bh
}

// dbCurrentBlockID is a convenience function allowing currentBlockID to be
// called without a bolt.Tx.
func (cs *ConsensusSet) dbCurrentBlockID() (id types.BlockID) {
	_ = cs.db.Update(func(tx *bolt.Tx) error {
		id = currentBlockID(tx)
		return nil
	})
	return id
}

// dbCurrentProcessedBlock is a convenience function allowing
// currentProcessedBlock to be called without a bolt.Tx.
func (cs *ConsensusSet) dbCurrentProcessedBlock() (pb *processedBlock) {
	_ = cs.db.Update(func(tx *bolt.Tx) error {
		pb = currentProcessedBlock(tx)
		return nil
	})
	return pb
}

// dbGetPath is a convenience function allowing getPath to be called without a
// bolt.Tx.
func (cs *ConsensusSet) dbGetPath(bh types.BlockHeight) (id types.BlockID) {
	_ = cs.db.Update(func(tx *bolt.Tx) error {
		id = getPath(tx, bh)
		return nil
	})
	return id
}

/// BREAK ///

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
			return commitDelayedSiacoinOutputDiff(tx, dscod, modules.DiffApply)
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
	err := cs.db.Update(func(tx *bolt.Tx) error {
		return commitFileContractDiff(tx, fcd, modules.DiffApply)
	})
	if err != nil {
		return err
	}

	return nil
}

// addDelayedSiacoinOutputsHeight inserts a siacoin output to the bucket at a particular height
func (db *setDB) addDelayedSiacoinOutputsHeight(h types.BlockHeight, id types.SiacoinOutputID, sco types.SiacoinOutput) {
	bucketID := append(prefix_dsco, encoding.Marshal(h)...)
	err := db.Update(func(tx *bolt.Tx) error {
		return insertItem(tx, bucketID, id, sco)
	})
	if err != nil {
		panic(err)
	}
}

// rmDelayedSiacoinOutputsHeight removes a siacoin output with a given ID at the given height
func (db *setDB) rmDelayedSiacoinOutputsHeight(h types.BlockHeight, id types.SiacoinOutputID) error {
	bucketID := append(prefix_dsco, encoding.Marshal(h)...)
	return db.rmItem(bucketID, id)
}

// lenSiacoinOutputs returns the size of the siacoin outputs bucket
func (db *setDB) lenSiacoinOutputs() uint64 {
	return db.lenBucket(SiacoinOutputs)
}

// lenFileContracts returns the number of file contracts in the consensus set
func (db *setDB) lenFileContracts() uint64 {
	return db.lenBucket(FileContracts)
}

// lenFCExpirationsHeight returns the number of file contracts which expire at a given height
func (db *setDB) lenFCExpirationsHeight(h types.BlockHeight) uint64 {
	bucketID := append(prefix_fcex, encoding.Marshal(h)...)
	return db.lenBucket(bucketID)
}

// lenSiafundOutputs returns the size of the SiafundOutputs bucket
func (db *setDB) lenSiafundOutputs() uint64 {
	return db.lenBucket(SiafundOutputs)
}
