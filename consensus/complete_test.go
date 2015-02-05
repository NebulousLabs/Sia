package consensus

import (
	"testing"
)

// TODO: Add the 100block waiting outputs to the currency tallying.

// TODO: Split some of these tests off into fork_test.go.

// TODO: in fork_test.go (which doesn't exist yet), try calling fork() on a
// node that's already in the current consensus set.

// currentPathCheck looks at every block listed in currentPath and verifies
// that every block from current to genesis matches the block listed in
// currentPath.
func currentPathCheck(t *testing.T, s *State) {
	currentNode := s.currentBlockNode()
	for i := s.Height(); i != 0; i-- {
		// Check that the CurrentPath entry exists.
		id, exists := s.currentPath[i]
		if !exists {
			t.Error("current path is empty for a height with a known block.")
		}

		// Check that the CurrentPath entry contains the correct block id.
		if currentNode.block.ID() != id {
			t.Error("current path does not have correct id!")
		}

		// Check that each parent is one less in height than its child.
		if currentNode.height != currentNode.parent.height+1 {
			t.Error("heights are messed up")
		}

		currentNode = s.blockMap[currentNode.block.ParentID]
	}
}

// rewindApplyCheck grabs the state hash and then rewinds to the genesis block.
// Then the state moves forwards to the initial starting place and verifies
// that the state hash is the same.
func rewindApplyCheck(t *testing.T, s *State) {
	stateHash := s.stateHash()
	rewoundNodes := s.rewindToNode(s.blockRoot)
	for i := len(rewoundNodes) - 1; i >= 0; i-- {
		s.applyBlockNode(rewoundNodes[i])
	}
	if stateHash != s.stateHash() {
		t.Error("state hash is not consistent after rewinding and applying all the way through")
	}
}

// currencyCheck uses the height to determine the total amount of currency that
// should be in the system, and then tallys up the outputs to see if that is
// the case.
func currencyCheck(t *testing.T, s *State) {
	siafunds := NewCurrency64(0)
	for _, siafundOutput := range s.unspentSiafundOutputs {
		siafunds.Add(siafundOutput.Value)
	}
	if siafunds.Cmp(NewCurrency64(SiafundCount)) != 0 {
		t.Error("siafunds inconsistency")
	}

	expectedSiacoins := NewCurrency64(0)
	for i := BlockHeight(0); i <= s.Height(); i++ {
		expectedSiacoins.Add(CalculateCoinbase(i))
	}
	siacoins := NewCurrency64(0)
	for _, output := range s.unspentOutputs {
		siacoins.Add(output.Value)
	}
	for _, contract := range s.openContracts {
		siacoins.Add(contract.Payout)
	}
	siacoins.Add(s.siafundPool)
	if siacoins.Cmp(expectedSiacoins) != 0 {
		t.Error(siacoins)
		t.Error(expectedSiacoins)
		t.Error("siacoins inconsistency")
	}
}

// consistencyChecks calls all of the consistency functions on each of the
// states.
func consistencyChecks(t *testing.T, states ...*State) {
	for _, s := range states {
		currentPathCheck(t, s)
		rewindApplyCheck(t, s)
		currencyCheck(t, s)
	}
}

// orderedTestBattery calls all of the individual tests on each of the input
// states. The goal is to produce state with consistent but diverse sets of
// blocks to more effectively test things like diffs and forking.
func orderedTestBattery(t *testing.T, states ...*State) {
	for _, s := range states {
		// blocks_test.go tests
		testBlockTimestamps(t, s)
		testEmptyBlock(t, s)
		testLargeBlock(t, s)
		testMinerPayouts(t, s)
		testMissedTarget(t, s)
		testRepeatBlock(t, s)

		// transactions_test.go tests
		testForeignSignature(t, s)
		testInvalidSignature(t, s)
		testSingleOutput(t, s)
		testUnsignedTransaction(t, s)

		// contracts_test.go tests
		testContractCreation(t, s)
		testMissedProof(t, s)
		testStorageProofSubmit(t, s)
	}
}

// TestEverything creates a state and uses that one state to perform all of the
// individual tests, building a sizeable state with a lot of diverse
// transactions. Then it performs consistency checks and other stress testing.
func TestEverything(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// To help test the forking code, we're creating two states. We'll start
	// each off on its own fork, and then test them together. They'll get the
	// same set of tests and be in the same place, except the only shared block
	// will be the genesis block. Then we'll mine blocks so that one is far
	// ahead of the other. We'll show all of the blocks to the other state,
	// which will cause it to fork and rewind the entire diverse set of blocks
	// and then apply an entirely different diverse set of blocks.
	genesisTime := currentTime() - 1
	s0 := CreateGenesisState(genesisTime)
	s1 := CreateGenesisState(genesisTime)

	// Verify that the genesis state creation is consistent.
	if s0.stateHash() != s1.stateHash() {
		t.Fatal("state hashes are different after calling CreateGenesisState")
	}

	// Get each on a separate fork.
	b0, err := mineTestingBlock(s0.CurrentBlock().ID(), currentTime()-1, nullMinerPayouts(s0.Height()+1), nil, s0.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s0.AcceptBlock(b0)
	if err != nil {
		t.Fatal(err)
	}
	b1, err := mineTestingBlock(s1.CurrentBlock().ID(), currentTime(), nullMinerPayouts(s1.Height()+1), nil, s1.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s1.AcceptBlock(b1)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that each state is on a separate fork.
	if s0.stateHash() == s1.stateHash() {
		t.Fatal("states have the same hash when they need to be in different places")
	}

	// Call orderedTestBattery on each state.
	orderedTestBattery(t, s0, s1)

	// Verify that each state is still on a separate fork.
	if s0.stateHash() == s1.stateHash() {
		t.Fatal("states have the same hash when they need to be in different places")
	}

	// Now perform consistency checks on each state.
	consistencyChecks(t, s0, s1)

	// Show all s0 blocks to s1, which should not trigger a fork.
	for i := BlockHeight(1); i <= s0.Height(); i++ {
		blockID := s0.currentPath[i]
		err = s1.AcceptBlock(s0.blockMap[blockID].block)
		if err != nil {
			t.Error(i, "::", blockID, "::", err)
		}
	}

	// Verify that each state is still on a separate fork.
	if s0.stateHash() == s1.stateHash() {
		t.Fatal("states have the same hash when they need to be in different places")
	}

	// Mine a block on s0 which spends a siacoin output which doesn't exist,
	// then show that block to s1. This should cause an error, but should also
	// result in the state verifying all of the s0 blocks and then
	// backtracking. Make sure the hash didn't shift during the backtracking.
	s1Hash := s1.stateHash()
	badID := OutputID{1, 2, 3}
	input := SiacoinInput{
		OutputID:        badID,
		SpendConditions: SpendConditions{},
	}
	badTxn := Transaction{
		SiacoinInputs:  []SiacoinInput{input},
		SiacoinOutputs: []SiacoinOutput{SiacoinOutput{}},
	}
	b, err := mineTestingBlock(s0.CurrentBlock().ID(), currentTime(), nullMinerPayouts(s0.Height()+1), []Transaction{badTxn}, s0.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s1.AcceptBlock(b)
	if err != MissingOutputErr {
		t.Error(err)
	}

	// Verify that after rejecting the block, the state hash hasn't changed.
	if s1.stateHash() != s1Hash {
		t.Error("s1 hash changed after rejecting a block")
	}

	// Verify that the diffs for the s0 current block were generated, meaning
	// that s1 did actually try to fork.
	bn := s0.blockMap[s0.CurrentBlock().ID()]
	if !bn.diffsGenerated {
		t.Error("s0 current block diffs not generated after attempting a fork")
	}

	// Create valid block on s0 and show it to both s0 and s1. This should
	// cause each to switch to that block as the most recent block, meaning
	// each has the same state hash.
	b, err = mineValidBlock(s0)
	if err != nil {
		t.Fatal(err)
	}
	err = s0.AcceptBlock(b)
	if err != nil {
		t.Error(err)
	}
	err = s1.AcceptBlock(b)
	if err != nil {
		t.Error(err)
	}

	// Check that s0 and s1 now have the same state hash
	if s0.stateHash() != s1.stateHash() {
		t.Error("state hashes do not match even though a fork should have occured")
	}

	// Perform consistency checks on s1.
	currentPathCheck(t, s1)
	rewindApplyCheck(t, s1)
}
