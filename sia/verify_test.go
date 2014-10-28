package sia

import (
	"testing"
)

func TestBlockBuilding(t *testing.T) {
	// Generate the Genesis State
	state := CreateGenesisState()

	// Create an empty second block.
	secondBlock := state.GenerateBlock()

	// Add the block to the state.
	err := state.AcceptBlock(secondBlock)
	if err != nil {
		t.Fatal(err)
	}

	// Add a transaction to the transaction pool.

	// Create a thrid block containing the transaction, add it.

	// Create a block with multiple transactions, but one isn't valid.
	// This will see if the reverse code works correctly.
}
