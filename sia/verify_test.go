package sia

import (
	"testing"
)

func TestBlockBuilding(t *testing.T) {
	state := CreateGenesisState()

	// Second block because CreateGenesisState includes the genesis block
	secondBlock := state.GenerateBlock()

	err := state.AcceptBlock(secondBlock)
	if err != nil {
		t.Fatal(err)
	}
}
