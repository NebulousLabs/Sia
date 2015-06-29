package consensus

import (
	"errors"
	"fmt"
	"sort"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// checkCurrentPath looks at the blocks in the current path and verifies that
// they are all ordered correctly and in the block map.
func (cs *State) checkCurrentPath() error {
	currentNode := cs.currentBlockNode()
	for i := cs.height(); i != 0; i-- {
		// The block should be in the block map.
		_, exists := cs.blockMap[currentNode.block.ID()]
		if !exists {
			return errors.New("current path block not found in block map")
		}
		// Current node should match the id in the current path.
		if currentNode.block.ID() != cs.currentPath[i] {
			return errors.New("current path points to an incorrect block")
		}
		// Height of node needs to be listed correctly.
		if currentNode.height != i {
			return errors.New("node height mismatches its location in the blockchain")
		}
		// Current node's parent needs the right id.
		if currentNode.block.ParentID != currentNode.parent.block.ID() {
			return errors.New("node parent id mismatches actual parent id")
		}

		currentNode = currentNode.parent
	}
	return nil
}

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

// checkSiacoins counts the number of siacoins in the database and verifies
// that it matches the sum of all the coinbases.
func (cs *State) checkSiacoins() error {
	expectedSiacoins := types.ZeroCurrency
	for i := types.BlockHeight(0); i <= cs.height(); i++ {
		expectedSiacoins = expectedSiacoins.Add(types.CalculateCoinbase(i))
	}
	totalSiacoins := types.ZeroCurrency
	for _, sco := range cs.siacoinOutputs {
		totalSiacoins = totalSiacoins.Add(sco.Value)
	}
	for _, fc := range cs.fileContracts {
		totalSiacoins = totalSiacoins.Add(fc.Payout)
	}
	for _, dsoMap := range cs.delayedSiacoinOutputs {
		for _, dso := range dsoMap {
			totalSiacoins = totalSiacoins.Add(dso.Value)
		}
	}
	totalSiacoins = totalSiacoins.Add(cs.siafundPool)
	if expectedSiacoins.Cmp(totalSiacoins) != 0 {
		return fmt.Errorf("checkSiacoins: expected %v siacoins, got %v siacoins", expectedSiacoins, totalSiacoins)
	}
	return nil
}

// checkSiafunds counts the siafund outputs and checks that there are 10,000.
func (cs *State) checkSiafunds() error {
	totalSiafunds := types.ZeroCurrency
	for _, sfo := range cs.siafundOutputs {
		totalSiafunds = totalSiafunds.Add(sfo.Value)
	}
	if totalSiafunds.Cmp(types.NewCurrency64(types.SiafundCount)) != 0 {
		return fmt.Errorf("checkSiafunds: expected %v siafunds, got %v siafunds", types.SiafundCount, totalSiafunds)
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
	// 6.	current path + diffs
	// 7.	earliest allowed timestamp of next block
	// 8.	unspent siacoin outputs, sorted by id.
	// 9.	open file contracts, sorted by id.
	// 10.	unspent siafund outputs, sorted by id.
	// 11.	delayed siacoin outputs, sorted by height, then sorted by id.
	// 12.	siafund pool

	// Create a slice of hashes representing all items of interest.
	tree := crypto.NewTree()
	tree.PushObject(cs.blockRoot.block)
	tree.PushObject(cs.height())
	tree.PushObject(cs.currentBlockNode().childTarget)
	tree.PushObject(cs.currentBlockNode().depth)
	tree.PushObject(cs.currentBlockNode().earliestChildTimestamp())

	// Add all the blocks in the current path TODO: along with their diffs.
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

	// Add the siafund pool
	tree.PushObject(cs.siafundPool)

	return tree.Root()
}

// checkRewindApply rewinds and reapplies the current block, checking that the
// consensus set hashes meet the expected values.
func (cs *State) checkRewindApply() error {
	// Do nothing if the DEBUG flag is not set.
	if !build.DEBUG {
		return nil
	}

	// Rewind a block, check that the consensus set hash after rewiding is the
	// same as it was before the current block was added, then reapply the
	// block and check that the new consensus set has is the same as originally
	// calculated.
	currentNode := cs.currentBlockNode()
	cs.revertToNode(currentNode.parent)
	if cs.consensusSetHash() != currentNode.parent.consensusSetHash {
		return errors.New("rewinding a block resulted in unexpected consensus set hash")
	}
	cs.applyUntilNode(currentNode)
	if cs.consensusSetHash() != currentNode.consensusSetHash {
		return errors.New("reapplying a block resulted in unexpected consensus set hash")
	}
	return nil
}

// checkConsistency runs a series of checks to make sure that the consensus set
// is consistent with some rules that should always be true.
func (cs *State) checkConsistency() error {
	err := cs.checkCurrentPath()
	if err != nil {
		return err
	}
	err = checkDelayedSiacoinOutputMaps()
	if err != nil {
		return err
	}
	err = checkSiacoins()
	if err != nil {
		return err
	}
	err = checkSiafunds()
	if err != nil {
		return err
	}
	err = checkRewindApply()
	if err != nil {
		return err
	}
	return nil
}
