package consensus

import (
	"github.com/boltdb/bolt"

	"github.com/NebulousLabs/Sia/types"
)

// height returns the current height of the state.
func (cs *ConsensusSet) height() (bh types.BlockHeight) {
	_ = cs.db.Update(func(tx *bolt.Tx) error {
		bh = blockHeight(tx)
		return nil
	})
	return bh
}

// ChildTarget returns the target for the child of a block.
func (cs *ConsensusSet) ChildTarget(id types.BlockID) (target types.Target, exists bool) {
	_ = cs.db.View(func(tx *bolt.Tx) error {
		pb, err := getBlockMap(tx, id)
		if err != nil {
			return nil
		}
		target = pb.ChildTarget
		exists = true
		return nil
	})
	return target, exists
}

// EarliestChildTimestamp returns the earliest timestamp that the next block can
// have in order for it to be considered valid.
func (cs *ConsensusSet) EarliestChildTimestamp(id types.BlockID) (timestamp types.Timestamp, exists bool) {
	// Error is not checked because it does not matter.
	_ = cs.db.View(func(tx *bolt.Tx) error {
		pb, err := getBlockMap(tx, id)
		if err != nil {
			return err
		}
		timestamp = earliestChildTimestamp(tx.Bucket(BlockMap), pb)
		exists = true
		return nil
	})
	return timestamp, exists
}

// GenesisBlock returns the genesis block.
func (cs *ConsensusSet) GenesisBlock() types.Block {
	lockID := cs.mu.RLock()
	defer cs.mu.RUnlock(lockID)
	return cs.blockRoot.Block
}

// InCurrentPath returns true if the block presented is in the current path,
// false otherwise.
func (cs *ConsensusSet) InCurrentPath(id types.BlockID) (inPath bool) {
	lockID := cs.mu.RLock()
	defer cs.mu.RUnlock(lockID)

	_ = cs.db.View(func(tx *bolt.Tx) error {
		pb, err := getBlockMap(tx, id)
		if err != nil {
			inPath = false
			return nil
		}
		inPath = getPath(tx, pb.Height) == id
		return nil
	})
	return inPath
}

// StorageProofSegment returns the segment to be used in the storage proof for
// a given file contract.
func (cs *ConsensusSet) StorageProofSegment(fcid types.FileContractID) (index uint64, err error) {
	lockID := cs.mu.RLock()
	defer cs.mu.RUnlock(lockID)

	_ = cs.db.View(func(tx *bolt.Tx) error {
		index, err = storageProofSegment(tx, fcid)
		return nil
	})
	return index, err
}
