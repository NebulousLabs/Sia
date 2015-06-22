package consensus

import (
	"errors"
	"sort"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// checkDelayedSiacoinOutputMaps checks that the delayed siacoin output maps
// have the right number of maps at the right heights.
func (cs *State) checkDelayedSiacoinOutputMaps() error {
	expected := 0
	for i := cs.height() + 1; i <= cs.height()+types.MaturityDelay; i++ {
		if !(i > types.MaturityDelay) {
			continue
		}
		_, exists := cs.delayedSiacoinOutputs[i]
		if !exists {
			return errors.New("delayed siacoin outputs are in an inconsistent state")
		}
		expected++
	}
	if len(cs.delayedSiacoinOutputs) != expected {
		return errors.New("delayed siacoin outputs has too many maps")
	}

	return nil
}

// consensusSetHash returns the Merkle root of the current state of consensus.
func (cs *State) consensusSetHash() crypto.Hash {
	// Items of interest:
	// 1.	genesis block
	// 3.	current height
	// 4.	current target
	// 5.	current depth
	// 6.	earliest allowed timestamp of next block
	// 7.	current path, ordered by height.
	// 8.	unspent siacoin outputs, sorted by id.
	// 9.	open file contracts, sorted by id.
	// 10.	unspent siafund outputs, sorted by id.
	// 11.	delayed siacoin outputs, sorted by height, then sorted by id.
	// TODO: Add the diff set ?

	// Create a slice of hashes representing all items of interest.
	tree := crypto.NewTree()
	tree.PushObject(cs.blockRoot.block)
	tree.PushObject(cs.height())
	tree.PushObject(cs.currentBlockNode().childTarget)
	tree.PushObject(cs.currentBlockNode().depth)
	tree.PushObject(cs.currentBlockNode().earliestChildTimestamp())

	// Add all the blocks in the current path.
	for i := 0; i < len(cs.currentPath); i++ {
		tree.PushObject(cs.currentPath[types.BlockHeight(i)])
	}

	// Add all of the siacoin outputs, sorted by id.
	var openSiacoinOutputs crypto.HashSlice
	for siacoinOutputID, _ := range cs.siacoinOutputs {
		openSiacoinOutputs = append(openSiacoinOutputs, crypto.Hash(siacoinOutputID))
	}
	sort.Sort(openSiacoinOutputs)
	for _, id := range openSiacoinOutputs {
		sco, exists := cs.siacoinOutputs[types.SiacoinOutputID(id)]
		if !exists {
			panic("trying to push nonexistent siacoin output")
		}
		tree.PushObject(id)
		tree.PushObject(sco)
	}

	// Add all of the file contracts, sorted by id.
	var openFileContracts crypto.HashSlice
	for fileContractID, _ := range cs.fileContracts {
		openFileContracts = append(openFileContracts, crypto.Hash(fileContractID))
	}
	sort.Sort(openFileContracts)
	for _, id := range openFileContracts {
		// Sanity Check - file contract should exist.
		fc, exists := cs.fileContracts[types.FileContractID(id)]
		if !exists {
			panic("trying to push a nonexistent file contract")
		}
		tree.PushObject(id)
		tree.PushObject(fc)
	}

	// Add all of the siafund outputs, sorted by id.
	var openSiafundOutputs crypto.HashSlice
	for siafundOutputID, _ := range cs.siafundOutputs {
		openSiafundOutputs = append(openSiafundOutputs, crypto.Hash(siafundOutputID))
	}
	sort.Sort(openSiafundOutputs)
	for _, id := range openSiafundOutputs {
		sco, exists := cs.siafundOutputs[types.SiafundOutputID(id)]
		if !exists {
			panic("trying to push nonexistent siafund output")
		}
		tree.PushObject(id)
		tree.PushObject(sco)
	}

	// Get the set of delayed siacoin outputs, sorted by maturity height then
	// sorted by id and add them.
	for i := cs.height() + 1; i <= cs.height()+types.MaturityDelay; i++ {
		var delayedSiacoinOutputs crypto.HashSlice
		for id := range cs.delayedSiacoinOutputs[i] {
			delayedSiacoinOutputs = append(delayedSiacoinOutputs, crypto.Hash(id))
		}
		sort.Sort(delayedSiacoinOutputs)

		for _, delayedSiacoinOutputID := range delayedSiacoinOutputs {
			delayedSiacoinOutput, exists := cs.delayedSiacoinOutputs[i][types.SiacoinOutputID(delayedSiacoinOutputID)]
			if !exists {
				panic("trying to push nonexistent delayed siacoin output")
			}
			tree.PushObject(delayedSiacoinOutput)
			tree.PushObject(delayedSiacoinOutputID)
		}
	}

	return tree.Root()
}
