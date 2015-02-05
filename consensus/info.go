package consensus

import (
	"errors"
	"sort"

	"github.com/NebulousLabs/Sia/hash"
)

// Contains basic information about the state, but does not go into depth.
type StateInfo struct {
	CurrentBlock BlockID
	Height       BlockHeight
	Target       Target
}

// BlockAtHeight() returns the block from the current history at the
// input height.
func (s *State) blockAtHeight(height BlockHeight) (b Block, exists bool) {
	bn, exists := s.blockMap[s.currentPath[height]]
	if !exists {
		return
	}
	b = bn.block
	return
}

// currentBlockNode returns the node of the most recent block in the
// longest fork.
func (s *State) currentBlockNode() *blockNode {
	return s.blockMap[s.currentBlockID]
}

// CurrentBlockWeight() returns the weight of the current block in the
// heaviest fork.
func (s *State) currentBlockWeight() BlockWeight {
	return s.currentBlockNode().target.Inverse()
}

// depth returns the depth of the current block of the state.
func (s *State) depth() Target {
	return s.currentBlockNode().depth
}

// height returns the current height of the state.
func (s *State) height() BlockHeight {
	return s.blockMap[s.currentBlockID].height
}

// State.Output returns the Output associated with the id provided for input,
// but only if the output is a part of the utxo set.
func (s *State) output(id OutputID) (sco SiacoinOutput, exists bool) {
	sco, exists = s.unspentSiacoinOutputs[id]
	return
}

// Sorted UtxoSet returns all of the unspent transaction outputs sorted
// according to the numerical value of their id.
func (s *State) sortedUtxoSet() (sortedOutputs []SiacoinOutput) {
	// Get all of the outputs in string form and sort the strings.
	var unspentOutputStrings []string
	for outputID := range s.unspentSiacoinOutputs {
		unspentOutputStrings = append(unspentOutputStrings, string(outputID[:]))
	}
	sort.Strings(unspentOutputStrings)

	// Get the outputs in order according to their sorted string form.
	for _, utxoString := range unspentOutputStrings {
		var outputID OutputID
		copy(outputID[:], utxoString)
		output, _ := s.output(outputID)
		sortedOutputs = append(sortedOutputs, output)
	}
	return
}

// StateHash returns the markle root of the current state of consensus.
func (s *State) stateHash() hash.Hash {
	// Items of interest:
	// 1. CurrentBlockID
	// 2. Current Height
	// 3. Current Target
	// 4. Current Depth
	// 5. Earliest Allowed Timestamp of Next Block
	// 6. Genesis Block
	// 7. CurrentPath, ordered by height.
	// 8. UnspentOutputs, sorted by id.
	// 9. OpenContracts, sorted by id.

	// Create a slice of hashes representing all items of interest.
	leaves := []hash.Hash{
		hash.Hash(s.currentBlockID),
		hash.HashObject(s.height()),
		hash.HashObject(s.currentBlockNode().target),
		hash.HashObject(s.currentBlockNode().depth),
		hash.HashObject(s.currentBlockNode().earliestChildTimestamp()),
		hash.Hash(s.blockRoot.block.ID()),
	}

	// Add all the blocks in the current path.
	for i := 0; i < len(s.currentPath); i++ {
		leaves = append(leaves, hash.Hash(s.currentPath[BlockHeight(i)]))
	}

	// Sort the unspent outputs by the string value of their ID.
	sortedUtxos := s.sortedUtxoSet()

	// Add the unspent outputs in sorted order.
	for _, output := range sortedUtxos {
		leaves = append(leaves, hash.HashObject(output))
	}

	// Sort the open contracts by the string value of their ID.
	var openContractStrings []string
	for contractID := range s.openFileContracts {
		openContractStrings = append(openContractStrings, string(contractID[:]))
	}
	sort.Strings(openContractStrings)

	// Add the open contracts in sorted order.
	for _, stringContractID := range openContractStrings {
		var contractID FileContractID
		copy(contractID[:], stringContractID)
		leaves = append(leaves, hash.HashObject(s.openFileContracts[contractID]))
	}

	return hash.MerkleRoot(leaves)
}

// Block returns the block associated with the given id.
func (s *State) Block(id BlockID) (b Block, exists bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	node, exists := s.blockMap[id]
	if !exists {
		return
	}
	b = node.block
	return
}

// BlockAtHeight returns the block in the current fork found at `height`.
func (s *State) BlockAtHeight(height BlockHeight) (b Block, exists bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bn, exists := s.blockMap[s.currentPath[height]]
	if !exists {
		return
	}
	b = bn.block
	return
}

func (s *State) BlockOutputDiffs(id BlockID) (scods []SiacoinOutputDiff, err error) {
	node, exists := s.blockMap[id]
	if !exists {
		err = errors.New("requested an unknown block")
		return
	}
	if !node.diffsGenerated {
		err = errors.New("diffs have not been generated for the requested block.")
		return
	}
	scods = node.siacoinOutputDiffs
	return
}

// Contract returns a the contract associated with the input id, and whether
// the contract exists.
func (s *State) Contract(id FileContractID) (fc FileContract, exists bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	fc, exists = s.openFileContracts[id]
	if !exists {
		return
	}
	return
}

// CurrentBlock returns the highest block on the tallest fork.
func (s *State) CurrentBlock() Block {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentBlockNode().block
}

// CurrentTarget returns the target of the next block that needs to be
// submitted to the state.
func (s *State) CurrentTarget() Target {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentBlockNode().target
}

// EarliestLegalTimestamp returns the earliest legal timestamp of the next
// block - earlier timestamps will render the block invalid.
func (s *State) EarliestTimestamp() Timestamp {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentBlockNode().earliestChildTimestamp()
}

// State.Height() returns the height of the longest fork.
func (s *State) Height() BlockHeight {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.height()
}

// HeightOfBlock returns the height of the block with id `bid`.
func (s *State) HeightOfBlock(bid BlockID) (height BlockHeight, exists bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bn, exists := s.blockMap[bid]
	if !exists {
		return
	}
	height = bn.height
	return
}

// Output returns the output associated with an OutputID, returning an error if
// the output is not found.
func (s *State) Output(id OutputID) (output SiacoinOutput, exists bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.output(id)
}

// Sorted UtxoSet returns all of the unspent transaction outputs sorted
// according to the numerical value of their id.
func (s *State) SortedUtxoSet() []SiacoinOutput {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sortedUtxoSet()
}

// StateHash returns the markle root of the current state of consensus.
func (s *State) StateHash() hash.Hash {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stateHash()
}
