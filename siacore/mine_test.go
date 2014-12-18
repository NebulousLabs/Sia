package siacore

import (
	"testing"
)

// mineSingleBlock mines a single block and then uses the blocking function
// processBlock to integrate the block with the state.
func mineSingleBlock(t *testing.T, e *Environment) {
	block, target := e.blockForWork()
	found, err := e.solveBlock(block, target)
	for !found {
		found, err = e.solveBlock(block, target)
	}
	if err != nil {
		t.Error(err)
	}
}

// testEmptyBlock creates an emtpy block and submits it to the state.
func testEmptyBlock(t *testing.T, e *Environment) {
	// Check that the block will actually be empty.
	if len(e.state.TransactionPoolDump()) != 0 {
		t.Error("TransactionPoolDump is not of len 0")
		return
	}

	height := e.Height()
	mineSingleBlock(t, e)
	if height+1 != e.Height() {
		t.Errorf("height should have increased by one, went from %v to %v.", height, e.Height())
	}
}

/*
// testToggleMining tests that enabling and disabling mining works without
// problems.
func testToggleMining(te *testEnv) {
	// Check that mining is not already enabled.
	if te.e0.mining {
		te.t.Error("Miner is already mining! - testToggleMining prereqs not met!")
		return
	}

	// Enable mining for a second, which should be more than enough to mine a
	// block in the testing environment.
	prevHeight := te.e0.Height()
	te.e0.StartMining()
	time.Sleep(300 * time.Millisecond)
	te.e0.StopMining()
	time.Sleep(300 * time.Millisecond)

	// Test the height, wait another second (to allow an incorrectly running
	// miner to mine another block) and test the height again.
	newHeight := te.e0.Height()
	if newHeight == prevHeight {
		te.t.Error("height did not increase after mining for a second")
	}
	time.Sleep(300 * time.Millisecond)
	if te.e0.Height() != newHeight {
		te.t.Error("height still increasing after disabling mining...")
	}
}

// testDualMining has both environments mine at the same time, and then
// verifies that they maintain consistency.
func testDualMining(te *testEnv) {
	if te.e0.mining || te.e1.mining {
		te.t.Error("one of the miners is already mining - testDualMining prereqs failed!")
		return
	}

	// Enable mining on each miner for long enough that each should mine
	// multiple blocks. Then give the miners time to synchronize.
	te.e0.StartMining()
	te.e1.StartMining()
	time.Sleep(300 * time.Millisecond)
	te.e0.StopMining()
	te.e1.StopMining()
	time.Sleep(500 * time.Millisecond)

		// Compare the state hash for equality.
		info0 := te.e0.StateInfo()
		info1 := te.e1.StateInfo()

		if info0.StateHash != info1.StateHash {
			te.t.Error("state hashes do not match after dual mining.")
			return
		}
}
*/
