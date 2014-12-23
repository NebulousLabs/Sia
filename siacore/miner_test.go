package siacore

import (
	"testing"
)

// mineSingleBlock mines a single block and then uses the blocking function
// processBlock to integrate the block with the state.
func mineSingleBlock(t *testing.T, e *Environment) {
	_, found, err := e.miner.SolveBlock()
	for !found && err == nil {
		_, found, err = e.miner.SolveBlock()
	}
	if err != nil {
		t.Error(err)
	}
}
