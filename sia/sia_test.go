package sia

import (
	"errors"
	"fmt"
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
		err = fmt.Errorf("Genesis state should have a single upspent output, has", len(testEnv.state.ConsensusState.UnspentOutputs))
		return
	}

	return
}

// Creates an empty block and applies it to the state.
func addEmptyBlock(testEnv *testingEnvironment) (err error) {
	// Make sure that the block will actually be empty.
	if len(testEnv.state.ConsensusState.TransactionList) != 0 {
		err = errors.New("cannot add an empty block without an empty transaction pool.")
		return
	}

	// Generate a valid empty block using GenerateBlock.
	emptyBlock := testEnv.state.GenerateBlock(testEnv.wallets[0].CoinAddress)
	if len(emptyBlock.Transactions) != 0 {
		err = errors.New("failed to make an empty block...")
		return
	}

	expectedOutputs := len(testEnv.state.ConsensusState.UnspentOutputs) + 1
	err = testEnv.state.AcceptBlock(emptyBlock)
	if err != nil {
		return
	}
	if len(testEnv.state.ConsensusState.UnspentOutputs) != expectedOutputs {
		err = fmt.Errorf("Expecting", expectedOutputs, "outputs, got", len(testEnv.state.ConsensusState.UnspentOutputs), "outputs")
		return
	}

	return
}

// makeSpendingEnvironment spends coins from wallet0 into a set of new wallets,
// pushing through a block that will commit the transactions.
func makeSpendingEnvironment(testEnv *testingEnvironment) (err error) {
	// The current wallet design means that it will double spend on
	// sequential transactions - meaning that if you make two transactions
	// in the same block, the wallet will use the same input for each.
	// We'll fix this sooner rather than later, but for now the problem has
	// been left so we can focus on other things.

	// Create the new wallets that will be used for the spending
	// environment.
	for i := 0; i < 1; i++ {
		var wallet *Wallet
		wallet, err = CreateWallet()
		if err != nil {
			return
		}
		testEnv.wallets = append(testEnv.wallets, wallet)
	}

	// Create transactions that send coins to each wallet.
	for i := 1; i < 2; i++ {
		var transaction Transaction
		transaction, err = testEnv.wallets[0].SpendCoins(Currency(i+3), testEnv.wallets[len(testEnv.wallets)-i].CoinAddress, testEnv.state)
		if err != nil {
			return
		}
		err = testEnv.state.AcceptTransaction(&transaction)
		if err != nil {
			return
		}
	}

	return
}

// For now, this is really just a catch-all test. I'm not really sure how to
// modularize the various components =/
func TestBlockBuilding(t *testing.T) {
	// Initialize the testing evironment.
	testEnv, err := createEnvironment()
	if err != nil {
		t.Fatal(err)
	}

	// Add an empty block to the testing environment.
	err = addEmptyBlock(testEnv)
	if err != nil {
		t.Fatal(err)
	}

	// Create a few new wallets and send coins to each in a block.
	err = makeSpendingEnvironment(testEnv)
	if err != nil {
		t.Fatal(err)
	}

	// Create a third block with transactions.

	// Create a thrid block containing the transaction, add it.

	// Create a block with multiple transactions, but one isn't valid.
	// This will see if the reverse code works correctly.
}
