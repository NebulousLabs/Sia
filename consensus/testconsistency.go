package consensus

import (
	"sort"

	"github.com/NebulousLabs/Sia/crypto"
)

// CurrentPathCheck looks at every block listed in currentPath and verifies
// that every block from current to genesis matches the block listed in
// currentPath.
func (ct *ConsensusTester) CurrentPathCheck() {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	currentNode := ct.currentBlockNode()
	for i := ct.Height(); i != 0; i-- {
		// Check that the node has a corresponding entry in the blockMap
		if _, exists := ct.blockMap[currentNode.block.ID()]; !exists {
			ct.Error("currentPath has diverged from blockMap")
		}

		// Check that the CurrentPath entry contains the correct block ID.
		if currentNode.block.ID() != ct.currentPath[i] {
			ct.Error("current path does not have correct ID!")
		}

		// Check that the node's height is correct.
		if currentNode.height != currentNode.parent.height+1 {
			ct.Error("heights are messed up")
		}

		currentNode = currentNode.parent
	}
}

// rewindApplyCheck grabs the state hash and then rewinds to the genesis block.
// Then the state moves forwards to the initial starting place and verifies
// that the state hash is the same.
func (ct *ConsensusTester) RewindApplyCheck() {
	ct.mu.Lock()
	defer ct.mu.Unlock()

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
	ct.mu.RLock()
	defer ct.mu.RUnlock()

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

// stateHash returns the Merkle root of the current state of consensus.
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
	tree := crypto.NewTree()
	tree.PushObject(s.blockRoot.block)
	tree.PushObject(s.height())
	tree.PushObject(s.currentBlockNode().target)
	tree.PushObject(s.currentBlockNode().depth)
	tree.PushObject(s.currentBlockNode().earliestChildTimestamp())

	// Add all the blocks in the current path.
	for i := 0; i < len(s.currentPath); i++ {
		tree.PushObject(s.currentPath[BlockHeight(i)])
	}

	// Get the set of siacoin outputs in sorted order and add them.
	sortedUscos := s.sortedUscoSet()
	for _, output := range sortedUscos {
		tree.PushObject(output)
	}

	// Sort the open contracts by ID.
	var openContracts crypto.HashSlice
	for contractID := range s.fileContracts {
		openContracts = append(openContracts, crypto.Hash(contractID))
	}
	sort.Sort(openContracts)

	// Add the open contracts in sorted order.
	for _, id := range openContracts {
		tree.PushObject(id)
	}

	// Get the set of siafund outputs in sorted order and add them.
	for _, output := range s.sortedUsfoSet() {
		tree.PushObject(output)
	}

	// Get the set of delayed siacoin outputs, sorted by maturity height then
	// sorted by id and add them.
	for i := BlockHeight(0); i <= s.height(); i++ {
		var delayedOutputs crypto.HashSlice
		for id := range s.delayedSiacoinOutputs[i] {
			delayedOutputs = append(delayedOutputs, crypto.Hash(id))
		}
		sort.Sort(delayedOutputs)

		for _, output := range delayedOutputs {
			tree.PushObject(output)
		}
	}

	return tree.Root()
}

// StateHash returns the markle root of the current state of consensus.
func (s *State) StateHash() crypto.Hash {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.consensusSetHash()
}
