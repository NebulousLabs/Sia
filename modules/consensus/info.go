package consensus

import (
	"github.com/NebulousLabs/Sia/types"
)

// StateInfo contains basic information about the State.
type StateInfo struct {
	CurrentBlock types.BlockID
	Height       types.BlockHeight
	Target       types.Target
}

// currentBlockID returns the ID of the current block.
func (cs *State) currentBlockID() types.BlockID {
	return cs.currentPath[cs.height()]
}

// currentBlockNode returns the blockNode of the current block.
func (s *State) currentBlockNode() *blockNode {
	return s.blockMap[s.currentBlockID()]
}

// height returns the current height of the state.
func (s *State) height() types.BlockHeight {
	return types.BlockHeight(len(s.currentPath) - 1)
}

// CurrentBlock returns the highest block on the tallest fork.
func (s *State) CurrentBlock() types.Block {
	counter := s.mu.RLock()
	defer s.mu.RUnlock(counter)
	return s.currentBlockNode().block
}

// ChildTarget does not need a lock, as the values being read are not changed
// once they have been created.
func (s *State) ChildTarget(bid types.BlockID) (target types.Target, exists bool) {
	lockID := s.mu.RLock()
	defer s.mu.RUnlock(lockID)

	bn, exists := s.blockMap[bid]
	if !exists {
		return
	}
	target = bn.childTarget
	return
}

// EarliestChildTimestamp returns the earliest timestamp that the next block can
// have in order for it to be considered valid.
func (s *State) EarliestChildTimestamp(bid types.BlockID) (timestamp types.Timestamp, exists bool) {
	id := s.mu.RLock()
	defer s.mu.RUnlock(id)
	bn, exists := s.blockMap[bid]
	if !exists {
		return
	}
	timestamp = bn.earliestChildTimestamp()
	return
}

// GenesisBlock returns the genesis block.
func (s *State) GenesisBlock() types.Block {
	lockID := s.mu.RLock()
	defer s.mu.RUnlock(lockID)
	return s.blockMap[s.currentPath[0]].block
}

// Height returns the height of the current blockchain (the longest fork).
func (s *State) Height() types.BlockHeight {
	counter := s.mu.RLock()
	defer s.mu.RUnlock(counter)
	return s.height()
}

// InCurrentPath returns true if the block presented is in the current path,
// false otherwise.
func (s *State) InCurrentPath(bid types.BlockID) bool {
	lockID := s.mu.RLock()
	defer s.mu.RUnlock(lockID)

	node, exists := s.blockMap[bid]
	if !exists {
		return false
	}
	return s.currentPath[node.height] == bid
}

// StorageProofSegment returns the segment to be used in the storage proof for
// a given file contract.
func (cs *State) StorageProofSegment(fcid types.FileContractID) (index uint64, err error) {
	lockID := cs.mu.RLock()
	defer cs.mu.RUnlock(lockID)
	return cs.storageProofSegment(fcid)
}
