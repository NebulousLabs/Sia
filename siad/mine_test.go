package siad

import (
	"time"
)

// Takes an environment and mines until a single block is produced.
func (e *Environment) mineSingleBlock() {
	for {
		b, target := e.miner.blockForWork()
		if solveBlock(b, target) {
			e.miner.blockChan <- *b
			break
		}
	}
}

// testToggleMining tests that enabling and disabling mining works without
// problems.
func testToggleMining(te *testEnv) {
	prevHeight := te.e0.state.Height()

	// Check that mining is not already enabled.
	if te.e0.miner.mining {
		te.t.Error("Miner is already mining! - testToggleMining prereqs not met!")
		return
	}

	// Enable mining for a second, which should be more than enough to mine a
	// block in the testing environment.
	te.e0.ToggleMining()
	if !te.e0.miner.mining {
		te.t.Error("Miner is not reporting as mining after mining was enabled.")
	}
	time.Sleep(300 * time.Millisecond)
	te.e0.ToggleMining()
	if te.e0.miner.mining {
		te.t.Error("Miner is reporting as mining after mining was disabled.")
	}

	// Test the height, wait another second (to allow an incorrectly running
	// miner to mine another block) and test the height again.
	info := te.e0.SafeStateInfo()
	newHeight := info.Height
	if newHeight == prevHeight {
		te.t.Error("height did not increase after mining for a second")
	}
	time.Sleep(300 * time.Millisecond)
	info = te.e0.SafeStateInfo()
	if info.Height != newHeight {
		te.t.Error("height still increasing after disabling mining...")
	}
}

// testDualMining has both environments mine at the same time, and then
// verifies that they maintain consistency.
func testDualMining(te *testEnv) {
	if te.e0.miner.mining || te.e1.miner.mining {
		te.t.Error("one of the miners is already mining - testDualMining prereqs failed!")
		return
	}

	// Enable mining on each miner for long enough that each should mine
	// multiple blocks. Then give the miners time to synchronize.
	te.e0.ToggleMining()
	te.e1.ToggleMining()
	time.Sleep(300 * time.Millisecond)
	te.e0.ToggleMining()
	te.e1.ToggleMining()
	time.Sleep(300 * time.Millisecond)

	// Compare the state hash for equality.
	info0 := te.e0.SafeStateInfo()
	info1 := te.e1.SafeStateInfo()

	if info0.StateHash != info1.StateHash {
		te.t.Error("state hashes do not match after dual mining.")
		return
	}
}
