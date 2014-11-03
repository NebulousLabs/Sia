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
	err = testEnv.state.AcceptBlock(*emptyBlock)
	if err != nil {
		return
	}
	if len(testEnv.state.ConsensusState.UnspentOutputs) != expectedOutputs {
		err = fmt.Errorf("Expecting", expectedOutputs, "outputs, got", len(testEnv.state.ConsensusState.UnspentOutputs), "outputs")
		return
	}

	return
}

// transactionPoolTests adds a few wallets to the test environment, creating
// transactions that fund each and probes the overall efficiency of the
// transaction pool structures.
func transactionPoolTests(testEnv *testingEnvironment) (err error) {
	// The current wallet design means that it will double spend on
	// sequential transactions - meaning that if you make two transactions
	// in the same block, the wallet will use the same input for each.
	// We'll fix this sooner rather than later, but for now the problem has
	// been left so we can focus on other things.
	// Record the size of the transaction pool and the transaction list.

	// One thing we can do to increase the modularity of this function is
	// create a block at the beginning, and use the coinbase to create a
	// bunch of new wallets. This would also clear out the transaction pool
	// right at the beginning of the function.

	txnPoolLen := len(testEnv.state.ConsensusState.TransactionPool)
	txnListLen := len(testEnv.state.ConsensusState.TransactionList)

	// Create a new wallet for the test environment.
	wallet, err := CreateWallet()
	if err != nil {
		return
	}
	testEnv.wallets = append(testEnv.wallets, wallet)

	// Create a transaction to send to that wallet.
	transaction, err := testEnv.wallets[0].SpendCoins(Currency(3), testEnv.wallets[len(testEnv.wallets)-1].CoinAddress, testEnv.state)
	if err != nil {
		return
	}
	err = testEnv.state.AcceptTransaction(transaction)
	if err != nil {
		return
	}

	// Attempt to create a conflicting transaction and see if it is rejected from the pool.
	transaction.Outputs[0].SpendHash[0] = ^transaction.Outputs[0].SpendHash[0] // Change the output address
	transactionSigHash := transaction.SigHash(0)
	transaction.Signatures[0].Signature, err = SignBytes(transactionSigHash[:], testEnv.wallets[0].SecretKey) // Re-sign
	if err != nil {
		return
	}
	err = testEnv.state.AcceptTransaction(transaction)
	if err == nil {
		err = errors.New("Added a conflicting transaction to the transaction pool without error.")
		return
	}
	err = nil

	// The length of the transaction list should have grown by 1, and the
	// transaction pool should have grown by the number of outputs.
	if len(testEnv.state.ConsensusState.TransactionPool) != txnPoolLen+len(transaction.Inputs) {
		err = fmt.Errorf(
			"transaction pool did not grow by expected length. Started at %v and ended at %v but should have grown by %v",
			txnPoolLen,
			len(testEnv.state.ConsensusState.TransactionPool),
			len(transaction.Inputs),
		)
		return
	}
	if len(testEnv.state.ConsensusState.TransactionList) != txnListLen+1 {
		err = errors.New("transaction list did not grow by the expected length.")
		return
	}

	// Put a block through, which should clear the transaction pool
	// completely. Give the subsidy to the old wallet to replenish for
	// funding new wallets.
	transactionBlock := testEnv.state.GenerateBlock(testEnv.wallets[0].CoinAddress)
	if len(transactionBlock.Transactions) == 0 {
		err = errors.New("block created without accepting the transactions in the pool.")
		return
	}
	err = testEnv.state.AcceptBlock(*transactionBlock)
	if err != nil {
		return
	}

	// Check that the transaction pool has been cleared out.
	if len(testEnv.state.ConsensusState.TransactionPool) != 0 {
		err = errors.New("transaction pool not cleared out after getting a block.")
		return
	}
	if len(testEnv.state.ConsensusState.TransactionList) != 0 {
		err = errors.New("transaction list not cleared out after getting a block.")
		return
	}

	return
}

func blockForkingTests(testEnv *testingEnvironment) (err error) {
	// Fork from the current chain to a different chain, requiring a block
	// rewind.
	{
		// Create two blocks on the same parent.
		fork1a := testEnv.state.GenerateBlock(testEnv.wallets[0].CoinAddress) // A block along 1 fork
		fork2a := testEnv.state.GenerateBlock(testEnv.wallets[1].CoinAddress) // A block along a different fork.
		err = testEnv.state.AcceptBlock(*fork1a)
		if err != nil {
			return
		}

		// Add one block, mine on it to create a 'heaviest chain' and
		// then rewind the block, so that you can move the state along
		// the other chain.
		fork1b := testEnv.state.GenerateBlock(testEnv.wallets[0].CoinAddress) // Fork 1 is now heaviest
		testEnv.state.rewindABlock()                                          // Rewind to parent

		// Make fork2 the chosen fork.
		err = testEnv.state.AcceptBlock(*fork2a)
		if err != nil {
			return
		}
		// Verify that fork2a is the current block.
		if testEnv.state.ConsensusState.CurrentBlock != fork2a.ID() {
			err = errors.New("fork2 not accepted as farthest node.")
			return
		}

		// Add fork1b (rewinding does not remove a block from the
		// state) to the state and see if the forking happens.
		err = testEnv.state.AcceptBlock(*fork1b)
		if err != nil {
			return
		}
		// Verify that fork1b is the current block.
		if testEnv.state.ConsensusState.CurrentBlock != fork1b.ID() {
			err = errors.New("switching to a heavier chain did not appear to work.")
			return
		}
	}

	// Fork from the current chain to a different chain, but be required to
	// double back from validation problems.

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
	err = transactionPoolTests(testEnv)
	if err != nil {
		t.Fatal(err)
	}

	// Create a test that submits and removes transactions with multiple
	// inputs and outputs.

	// Test rewinding a block.

	err = blockForkingTests(testEnv)
	if err != nil {
		t.Fatal(err)
	}
}
