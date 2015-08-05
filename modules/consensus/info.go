package consensus

import (
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
func (s *ConsensusSet) height() types.BlockHeight {
	return s.blocksLoaded
}

// CurrentBlock returns the highest block on the tallest fork.
func (s *ConsensusSet) CurrentBlock() types.Block {
	counter := s.mu.RLock()
	defer s.mu.RUnlock(counter)
	return s.currentProcessedBlock().Block
}

// ChildTarget does not need a lock, as the values being read are not changed
// once they have been created.
//
// TODO: Right now, this function is only thread safe when called inside of
// 'ReceiveConsensusSetUpdate', but that should change once boltdb replaces the
// block mape
func (s *ConsensusSet) ChildTarget(bid types.BlockID) (target types.Target, exists bool) {
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
//
// TODO: Right now, this function is only thread safe when called inside of
// 'ReceiveConsensusSetUpdate', but that should change once boltdb replaces the
// block mape
func (s *ConsensusSet) EarliestChildTimestamp(bid types.BlockID) (timestamp types.Timestamp, exists bool) {
	exists = s.db.inBlockMap(bid)
	if !exists {
		return
	}
	pb := s.db.getBlockMap(bid)
	timestamp = s.earliestChildTimestamp(pb)
	return
}

// GenesisBlock returns the genesis block.
func (s *ConsensusSet) GenesisBlock() types.Block {
	lockID := s.mu.RLock()
	defer s.mu.RUnlock(lockID)
	return s.db.getBlockMap(s.db.getPath(0)).Block
}

// Height returns the height of the current blockchain (the longest fork).
func (s *ConsensusSet) Height() types.BlockHeight {
	counter := s.mu.RLock()
	defer s.mu.RUnlock(counter)
	return s.height()
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
	return cs.storageProofSegment(fcid)
}
