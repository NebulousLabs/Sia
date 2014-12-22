package siacore

import (
	"testing"
)

// mineSingleBlock mines a single block and then uses the blocking function
// processBlock to integrate the block with the state.
func mineSingleBlock(t *testing.T, e *Environment) {
	/*
		block, target := e.blockForWork()
		found, err := e.solveBlock(block, target)
		for !found {
			found, err = e.solveBlock(block, target)
		}
		if err != nil {
			t.Error(err)
		}
	*/
}
