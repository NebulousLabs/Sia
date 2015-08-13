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

// commitSiacoinOutputDiff applies or reverts a SiacoinOutputDiff.
func (cs *ConsensusSet) commitSiacoinOutputDiff(scod modules.SiacoinOutputDiff, dir modules.DiffDirection) {
	// Sanity check - should not be adding an output twice, or deleting an
	// output that does not exist.
	if build.DEBUG {
		exists := cs.db.inSiacoinOutputs(scod.ID)
		if exists == (scod.Direction == dir) {
			panic(errBadCommitSiacoinOutputDiff)
		}
	}

	if scod.Direction == dir {
		cs.db.addSiacoinOutputs(scod.ID, scod.SiacoinOutput)
	} else {
		cs.db.rmSiacoinOutputs(scod.ID)
	}
}

// commitDelayedSiacoinOutputDiff applies or reverts a delayedSiacoinOutputDiff.
func (cs *ConsensusSet) commitDelayedSiacoinOutputDiff(dscod modules.DelayedSiacoinOutputDiff, dir modules.DiffDirection) {
	if dscod.Direction == dir {
		cs.db.addDelayedSiacoinOutputsHeight(dscod.MaturityHeight, dscod.ID, dscod.SiacoinOutput)
	}
	cs.db.rmDelayedSiacoinOutputsHeight(dscod.MaturityHeight, dscod.ID)
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
