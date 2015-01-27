package consensus

import (
	"sort"

	"github.com/NebulousLabs/Sia/hash"
)

// Sorted UtxoSet returns all of the unspent transaction outputs sorted
// according to the numerical value of their id.
func (s *State) sortedUtxoSet() (sortedOutputs []Output) {
	var unspentOutputStrings []string
	for outputID := range s.unspentOutputs {
		unspentOutputStrings = append(unspentOutputStrings, string(outputID[:]))
	}
	sort.Strings(unspentOutputStrings)

	for _, utxoString := range unspentOutputStrings {
		var outputID OutputID
		copy(outputID[:], utxoString)
		output, exists := s.output(outputID)
		if !exists {
			panic("output doesn't exist?")
		}
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
		hash.HashObject(s.currentBlockNode().Target),
		hash.HashObject(s.currentBlockNode().Depth),
		hash.HashObject(s.currentBlockNode().earliestChildTimestamp()),
		hash.Hash(s.blockRoot.Block.ID()),
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
	for contractID := range s.openContracts {
		openContractStrings = append(openContractStrings, string(contractID[:]))
	}
	sort.Strings(openContractStrings)

	// Add the open contracts in sorted order.
	for _, stringContractID := range openContractStrings {
		var contractID ContractID
		copy(contractID[:], stringContractID)
		leaves = append(leaves, hash.HashObject(s.openContracts[contractID]))
	}

	return hash.MerkleRoot(leaves)
}

// Sorted UtxoSet returns all of the unspent transaction outputs sorted
// according to the numerical value of their id.
func (s *State) SortedUtxoSet() []Output {
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
