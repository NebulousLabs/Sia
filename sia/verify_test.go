package sia

import (
	"testing"
)

// For now, this is really just a catch-all test. I'm not really sure how to
// modularize the various components =/
func TestBlockBuilding(t *testing.T) {
	wallet, err := CreateWallet()
	if err != nil {
		t.Fatal(err)
	}

	// Generate the Genesis State
	state := CreateGenesisState(wallet.CoinAddress)

	// Create an empty second block.
	secondBlock := wallet.GenerateBlock(state)

	// Add the block to the state.
	err = state.AcceptBlock(secondBlock)
	if err != nil {
		t.Fatal(err)
	}

	// Add a transaction to the transaction pool.

	// Create a thrid block containing the transaction, add it.

	// Create a block with multiple transactions, but one isn't valid.
	// This will see if the reverse code works correctly.
}
