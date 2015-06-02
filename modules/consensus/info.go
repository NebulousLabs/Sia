package consensus

import (
	"sort"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// StateInfo contains basic information about the State.
type StateInfo struct {
	CurrentBlock types.BlockID
	Height       types.BlockHeight
	Target       types.Target
}

// currentBlockID returns the ID of the current block.
func (s *State) currentBlockID() types.BlockID {
	return s.currentPath[s.height()]
}

// currentBlockNode returns the blockNode of the current block.
func (s *State) currentBlockNode() *blockNode {
	return s.blockMap[s.currentBlockID()]
}

// height returns the current height of the state.
func (s *State) height() types.BlockHeight {
	return types.BlockHeight(len(s.currentPath) - 1)
}

// output returns the unspent SiacoinOutput associated with the given ID. If
// the output is not in the UTXO set, 'exists' will be false.
func (s *State) output(id types.SiacoinOutputID) (sco types.SiacoinOutput, exists bool) {
	sco, exists = s.siacoinOutputs[id]
	return
}

// sortedUscoSet returns all of the unspent siacoin outputs sorted
// according to the numerical value of their id.
func (s *State) sortedUscoSet() []types.SiacoinOutput {
	// Get all of the outputs in string form and sort the strings.
	unspentOutputs := make(crypto.HashSlice, len(s.siacoinOutputs))
	for outputID := range s.siacoinOutputs {
		unspentOutputs = append(unspentOutputs, crypto.Hash(outputID))
	}
	sort.Sort(unspentOutputs)

	// Get the outputs in order according to their sorted form.
	sortedOutputs := make([]types.SiacoinOutput, len(unspentOutputs))
	for i, outputID := range unspentOutputs {
		output, _ := s.output(types.SiacoinOutputID(outputID))
		sortedOutputs[i] = output
	}
	return sortedOutputs
}

// Sorted UsfoSet returns all of the unspent siafund outputs sorted according
// to the numerical value of their id.
func (s *State) sortedUsfoSet() []types.SiafundOutput {
	// Get all of the outputs in string form and sort the strings.
	outputIDs := make(crypto.HashSlice, len(s.siafundOutputs))
	for outputID := range s.siafundOutputs {
		outputIDs = append(outputIDs, crypto.Hash(outputID))
	}
	sort.Sort(outputIDs)

	// Get the outputs in order according to their sorted string form.
	sortedOutputs := make([]types.SiafundOutput, len(outputIDs))
	for i, outputID := range outputIDs {
		// Sanity check - the output should exist.
		output, exists := s.siafundOutputs[types.SiafundOutputID(outputID)]
		if build.DEBUG {
			if !exists {
				panic("output doesn't exist")
			}
		}

		sortedOutputs[i] = output
	}
	return sortedOutputs
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
func (s *State) StorageProofSegment(fcid types.FileContractID) (index uint64, err error) {
	counter := s.mu.RLock()
	defer s.mu.RUnlock(counter)
	return s.storageProofSegment(fcid)
}
