package sia

import (
	"testing"
)

// mineSingleBlock mines a single block and then uses the blocking function
// processBlock to integrate the block with the state.
func mineSingleBlock(t *testing.T, e *Environment) {
	b, found, err := e.miner.SolveBlock()
	for !found && err == nil {
		b, found, err = e.miner.SolveBlock()
	}
	if err != nil {
		t.Error(err)
	}
	err = e.processBlock(b)
	if err != nil {
		t.Error(err)
	}
}
