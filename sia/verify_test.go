package sia

import (
	"errors"
	"testing"
)

type testingEnvironment struct {
	wallets []*Wallet
	state   *State
}

// Creates a wallet and a state to use for testing.
func createEnvironment() (testEnv *testingEnvironment, err error) {
	testEnv = new(testingEnvironment)

	firstWallet, err := CreateWallet()
	if err != nil {
		return
	}
	testEnv.wallets = append(testEnv.wallets, firstWallet)

	testEnv.state = CreateGenesisState(testEnv.wallets[0].CoinAddress)

	if len(testEnv.state.ConsensusState.UnspentOutputs) != 1 {
		println(len(testEnv.state.ConsensusState.UnspentOutputs))
		err = errors.New("Genesis state should have a single unspent output.")
		return
	}

	return
}

// For now, this is really just a catch-all test. I'm not really sure how to
// modularize the various components =/
func TestBlockBuilding(t *testing.T) {
	testEnv, err := createEnvironment()
	if err != nil {
		t.Fatal(err)
	}

	// Create an empty second block.
	secondBlock := testEnv.state.GenerateBlock(testEnv.wallets[0].CoinAddress)

	// Add the block to the state.
	err = testEnv.state.AcceptBlock(secondBlock)
	if err != nil {
		t.Fatal(err)
	}

	// Create a third block with transactions.

	// Add a transaction to the transaction pool.

	// Create a thrid block containing the transaction, add it.

	// Create a block with multiple transactions, but one isn't valid.
	// This will see if the reverse code works correctly.
}
