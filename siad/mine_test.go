package siad

import (
	"time"
)

func testToggleMining(te *testEnv) {
	prevHeight := te.e0.state.Height()

	// Enable mining for a second, which should be more than enough to mine a
	// block in the testing environment.
	te.e0.ToggleMining()
	time.Sleep(1 * time.Second)
	te.e0.ToggleMining()

	// Test the height, wait another second (to allow an incorrectly running
	// miner to mine another block) and test the height again.
	info := te.e0.SafeStateInfo()
	newHeight := info.Height
	if newHeight == prevHeight {
		te.t.Error("height did not increase after mining for a second")
	}
	time.Sleep(1 * time.Second)
	info = te.e0.SafeStateInfo()
	if info.Height != newHeight {
		te.t.Error("height still increasing after disabling mining...")
	}
}
