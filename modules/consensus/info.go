package consensus

import (
	"github.com/boltdb/bolt"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

// ConsensusSetInfo contains basic information about the ConsensusSet.
type ConsensusSetInfo struct {
	CurrentBlock types.BlockID
	Height       types.BlockHeight
	Target       types.Target
}

// currentBlockID returns the ID of the current block.
func (cs *ConsensusSet) currentBlockID() types.BlockID {
	return cs.db.getPath(cs.height())
}

func (cs *ConsensusSet) currentProcessedBlock() *processedBlock {
	return cs.db.getBlockMap(cs.currentBlockID())
}

// height returns the current height of the state.
func (cs *ConsensusSet) height() (bh types.BlockHeight) {
	_ = cs.db.Update(func(tx *bolt.Tx) error {
		bh = blockHeight(tx)
		return nil
	})
	return bh
}

// CurrentBlock returns the highest block on the tallest fork.
func (s *ConsensusSet) CurrentBlock() types.Block {
	counter := s.mu.RLock()
	defer s.mu.RUnlock(counter)
	return s.currentProcessedBlock().Block
}

// ChildTarget returns the target for the child of a block.
func (s *ConsensusSet) ChildTarget(bid types.BlockID) (target types.Target, exists bool) {
	// Lock is not needed because the values being read will not change once
	// they have been created.
	exists = s.db.inBlockMap(bid)
	if !exists {
		return
	}
	pb := s.db.getBlockMap(bid)
	target = pb.ChildTarget
	return
}

// EarliestChildTimestamp returns the earliest timestamp that the next block can
// have in order for it to be considered valid.
func (cs *ConsensusSet) EarliestChildTimestamp(bid types.BlockID) (timestamp types.Timestamp, exists bool) {
	// Lock is not needed because the values being read will not change once
	// they have been created.
	err := cs.db.View(func(tx *bolt.Tx) error {
		// Check that the parent exists.
		blockMap := tx.Bucket(BlockMap)

		// The identifier for the BlockMap is the sia encoding of the parent
		// id. The sia encoding is the same as ParentID[:].
		var parent processedBlock
		parentBytes := blockMap.Get(bid[:])
		if parentBytes == nil {
			return ErrOrphan
		}
		err := encoding.Unmarshal(parentBytes, &parent)
		if err != nil {
			return err
		}
		timestamp = earliestChildTimestamp(blockMap, &parent)
		return nil
	})
	if err != nil {
		return 0, false
	}
	return timestamp, true
}

// GenesisBlock returns the genesis block.
func (s *ConsensusSet) GenesisBlock() types.Block {
	lockID := s.mu.RLock()
	defer s.mu.RUnlock(lockID)
	return s.db.getBlockMap(s.db.getPath(0)).Block
}

// InCurrentPath returns true if the block presented is in the current path,
// false otherwise.
func (s *ConsensusSet) InCurrentPath(bid types.BlockID) bool {
	lockID := s.mu.RLock()
	defer s.mu.RUnlock(lockID)

	exists := s.db.inBlockMap(bid)
	if !exists {
		return false
	}
	node := s.db.getBlockMap(bid)
	return s.db.getPath(node.Height) == bid
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
