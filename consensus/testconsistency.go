package consensus

import (
	"sort"

	"github.com/NebulousLabs/Sia/crypto"
)

// CurrentPathCheck looks at every block listed in currentPath and verifies
// that every block from current to genesis matches the block listed in
// currentPath.
func (ct *ConsensusTester) CurrentPathCheck() {
	currentNode := ct.currentBlockNode()
	for i := ct.Height(); i != 0; i-- {
		// Check that the CurrentPath entry exists.
		id, exists := ct.currentPath[i]
		if !exists {
			ct.Error("current path is empty for a height with a known block.")
		}

		// Check that the CurrentPath entry contains the correct block id.
		if currentNode.block.ID() != id {
			ct.Error("current path does not have correct id!")
		}

		// Check that each parent is one less in height than its child.
		if currentNode.height != currentNode.parent.height+1 {
			ct.Error("heights are messed up")
		}

		currentNode = ct.blockMap[currentNode.block.ParentID]
	}
}

// rewindApplyCheck grabs the state hash and then rewinds to the genesis block.
// Then the state moves forwards to the initial starting place and verifies
// that the state hash is the same.
func (ct *ConsensusTester) RewindApplyCheck() {
	csh := ct.consensusSetHash()
	cn := ct.currentBlockNode()
	ct.rewindToNode(ct.blockRoot)
	ct.applyUntilNode(cn)
	if csh != ct.consensusSetHash() {
		ct.Error("state hash is not consistent after rewinding and applying all the way through")
	}
}

// currencyCheck uses the height to determine the total amount of currency that
// should be in the system, and then tallys up the outputs to see if that is
// the case.
func (ct *ConsensusTester) CurrencyCheck() {
	siafunds := NewCurrency64(0)
	for _, siafundOutput := range ct.siafundOutputs {
		siafunds = siafunds.Add(siafundOutput.Value)
	}
	if siafunds.Cmp(NewCurrency64(SiafundCount)) != 0 {
		ct.Error("siafunds inconsistency")
	}

	expectedSiacoins := NewCurrency64(0)
	for i := BlockHeight(0); i <= ct.Height(); i++ {
		expectedSiacoins = expectedSiacoins.Add(CalculateCoinbase(i))
	}
	siacoins := ct.siafundPool
	for _, output := range ct.siacoinOutputs {
		siacoins = siacoins.Add(output.Value)
	}
	for _, contract := range ct.fileContracts {
		siacoins = siacoins.Add(contract.Payout)
	}
	for height, dsoMap := range ct.delayedSiacoinOutputs {
		if height+MaturityDelay > ct.Height() {
			for _, dso := range dsoMap {
				siacoins = siacoins.Add(dso.Value)
			}
		}
	}
	if siacoins.Cmp(expectedSiacoins) != 0 {
		ct.Error(siacoins.i.String())
		ct.Error(expectedSiacoins.i.String())
		ct.Error("siacoins inconsistency")
	}
}

// consistencyChecks calls all of the consistency functions on each of the
// states.
func (ct *ConsensusTester) ConsistencyChecks() {
	ct.CurrentPathCheck()
	ct.RewindApplyCheck()
	ct.CurrencyCheck()
}

// stateHash returns the markle root of the current state of consensus.
func (s *State) consensusSetHash() crypto.Hash {
	// Items of interest:
	// 1.	genesis block
	// 2.	current block id
	// 3.	current height
	// 4.	current target
	// 5.	current depth
	// 6.	earliest allowed timestamp of next block
	// 7.	current path, ordered by height.
	// 8.	unspent siacoin outputs, sorted by id.
	// 9.	open file contracts, sorted by id.
	// 10.	unspent siafund outputs, sorted by id.
	// 11.	delayed siacoin outputs, sorted by height, then sorted by id.

	// Create a slice of hashes representing all items of interest.
	leaves := []crypto.Hash{
		crypto.HashObject(s.blockRoot.block),
		crypto.Hash(s.currentBlockID),
		crypto.HashObject(s.height()),
		crypto.HashObject(s.currentBlockNode().target),
		crypto.HashObject(s.currentBlockNode().depth),
		crypto.HashObject(s.currentBlockNode().earliestChildTimestamp()),
	}

	// Add all the blocks in the current path.
	for i := 0; i < len(s.currentPath); i++ {
		leaves = append(leaves, crypto.Hash(s.currentPath[BlockHeight(i)]))
	}

	// Get the set of siacoin outputs in sorted order and add them.
	sortedUscos := s.sortedUscoSet()
	for _, output := range sortedUscos {
		leaves = append(leaves, crypto.HashObject(output))
	}

	// Sort the open contracts by the string value of their ID.
	var openContractStrings []string
	for contractID := range s.fileContracts {
		openContractStrings = append(openContractStrings, string(contractID[:]))
	}
	sort.Strings(openContractStrings)

	// Add the open contracts in sorted order.
	for _, stringContractID := range openContractStrings {
		var contractID FileContractID
		copy(contractID[:], stringContractID)
		leaves = append(leaves, crypto.HashObject(s.fileContracts[contractID]))
	}

	// Get the set of siafund outputs in sorted order and add them.
	sortedUsfos := s.sortedUsfoSet()
	for _, output := range sortedUsfos {
		leaves = append(leaves, crypto.HashObject(output))
	}

	// Get the set of delayed siacoin outputs, sorted by maturity height then
	// sorted by id and add them.
	for i := BlockHeight(0); i <= s.height(); i++ {
		delayedOutputs := s.delayedSiacoinOutputs[i]
		var delayedStrings []string
		for id := range delayedOutputs {
			delayedStrings = append(delayedStrings, string(id[:]))
		}
		sort.Strings(delayedStrings)

		for _, delayedString := range delayedStrings {
			var id SiacoinOutputID
			copy(id[:], delayedString)
			leaves = append(leaves, crypto.HashObject(delayedOutputs[id]))
		}
	}

	return crypto.MerkleRoot(leaves)
}

// StateHash returns the markle root of the current state of consensus.
func (s *State) StateHash() crypto.Hash {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.consensusSetHash()
}
