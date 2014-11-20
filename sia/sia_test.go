package sia

import (
	"fmt"
	"testing"

	"github.com/NebulousLabs/Andromeda/signatures"
)

// testingEnvironment() is a struc that contains a state and a list of wallets,
// as well as a testing.T object. This creates a variable that can be passed to
// functions during testing to perform various the various tests on the
// codebase in a more functional way.
type testingEnvironment struct {
	t       *testing.T
	wallets []*Wallet
	state   *State
}

// createEnvironment() creates the genesis state and returns it in a testing
// environment that has a single wallet that has claimed all of the premined
// funds.
func createEnvironment(t *testing.T) (testEnv *testingEnvironment) {
	testEnv = new(testingEnvironment)
	testEnv.t = t

	firstWallet, err := CreateWallet()
	if err != nil {
		testEnv.t.Fatal(err)
	}
	testEnv.wallets = append(testEnv.wallets, firstWallet)

	testEnv.state = CreateGenesisState()
	testEnv.state.Server, err = NewTCPServer(9989)
	if err != nil {
		testEnv.t.Fatal(err)
	}

	if len(testEnv.state.UnspentOutputs) != 1 {
		err = fmt.Errorf("Genesis state should have a single upspent output, has %v", len(testEnv.state.UnspentOutputs))
		testEnv.t.Fatal(err)
	}

	return
}

// addEmptyBlock() generates an empty block and inserts it into the state.
func addEmptyBlock(testEnv *testingEnvironment) {
	// Make sure that the block will actually be empty.
	if len(testEnv.state.TransactionList) != 0 {
		testEnv.t.Fatal("cannot add an empty block without an empty transaction pool.")
	}

	// Generate a valid empty block using GenerateBlock.
	emptyBlock := testEnv.state.GenerateBlock(testEnv.wallets[0].SpendConditions.CoinAddress())
	if len(emptyBlock.Transactions) != 0 {
		testEnv.t.Fatal("failed to make an empty block...")
	}

	// Get the state to accept the block, and then check that at least one new
	// unspent output has been added.
	expectedOutputs := len(testEnv.state.UnspentOutputs) + 1
	err := testEnv.state.AcceptBlock(*emptyBlock)
	if err != nil {
		testEnv.t.Fatal(err)
	}
	if len(testEnv.state.UnspentOutputs) != expectedOutputs {
		err := fmt.Errorf("Expecting %v outputs, got %v outputs", expectedOutputs, len(testEnv.state.UnspentOutputs))
		testEnv.t.Fatal(err)
	}
}

// transactionPoolTests adds a few wallets to the test environment, creating
// transactions that fund each and probes the overall efficiency of the
// transaction pool structures.
func transactionPoolTests(testEnv *testingEnvironment) {
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

	txnPoolLen := len(testEnv.state.TransactionPool)
	txnListLen := len(testEnv.state.TransactionList)

	// Create a new wallet for the test environment.
	wallet, err := CreateWallet()
	if err != nil {
		testEnv.t.Fatal(err)
	}
	testEnv.wallets = append(testEnv.wallets, wallet)

	// Create a transaction to send to that wallet.
	transaction, err := testEnv.wallets[0].SpendCoins(Currency(3), Currency(1), testEnv.wallets[len(testEnv.wallets)-1].SpendConditions.CoinAddress(), testEnv.state)
	if err != nil {
		testEnv.t.Fatal(err)
	}

	// Attempt to create a conflicting transaction and see if it is rejected from the pool.
	transaction.Outputs[0].SpendHash[0] = ^transaction.Outputs[0].SpendHash[0] // Change the output address
	transactionSigHash := transaction.SigHash(0)
	transaction.Signatures[0].Signature, err = signatures.SignBytes(transactionSigHash[:], testEnv.wallets[0].SecretKey) // Re-sign
	if err != nil {
		testEnv.t.Fatal(err)
	}
	err = testEnv.state.AcceptTransaction(transaction)
	if err == nil {
		testEnv.t.Fatal("Added a conflicting transaction to the transaction pool without error.")
	}
	err = nil

	// The length of the transaction list should have grown by 1, and the
	// transaction pool should have grown by the number of outputs.
	if len(testEnv.state.TransactionPool) != txnPoolLen+len(transaction.Inputs) {
		err = fmt.Errorf(
			"transaction pool did not grow by expected length. Started at %v and ended at %v but should have grown by %v",
			txnPoolLen,
			len(testEnv.state.TransactionPool),
			len(transaction.Inputs),
		)
		testEnv.t.Fatal(err)
	}
	if len(testEnv.state.TransactionList) != txnListLen+1 {
		testEnv.t.Fatal("transaction list did not grow by the expected length.")
	}

	// Put a block through, which should clear the transaction pool
	// completely. Give the subsidy to the old wallet to replenish for
	// funding new wallets.
	transactionBlock := testEnv.state.GenerateBlock(testEnv.wallets[0].SpendConditions.CoinAddress())
	if len(transactionBlock.Transactions) == 0 {
		testEnv.t.Fatal("block created without accepting the transactions in the pool.")
	}
	err = testEnv.state.AcceptBlock(*transactionBlock)
	if err != nil {
		testEnv.t.Fatal(err)
	}

	// Check that the transaction pool has been cleared out.
	if len(testEnv.state.TransactionPool) != 0 {
		testEnv.t.Fatal("transaction pool not cleared out after getting a block.")
	}
	if len(testEnv.state.TransactionList) != 0 {
		testEnv.t.Fatal("transaction list not cleared out after getting a block.")
	}
}

// blockForkingTests() creates two competing chains, and puts the state on the
// shortest of the chains. Then the longest is introduced to the state, causing
// the state to switch form one fork to the other. This is then repeated,
// except that the state realizes the second fork is invalid as it switches.
func blockForkingTests(testEnv *testingEnvironment) {
	// Create two blocks on the same parent.
	fork1a := testEnv.state.GenerateBlock(testEnv.wallets[0].SpendConditions.CoinAddress()) // A block along 1 fork
	fork2a := testEnv.state.GenerateBlock(testEnv.wallets[1].SpendConditions.CoinAddress()) // A block along a different fork.
	err := testEnv.state.AcceptBlock(*fork1a)
	if err != nil {
		testEnv.t.Fatal(err)
	}

	// Add one block, mine on it to create a 'heaviest chain' and
	// then rewind the block, so that you can move the state along
	// the other chain.
	fork1b := testEnv.state.GenerateBlock(testEnv.wallets[0].SpendConditions.CoinAddress()) // Fork 1 is now heaviest
	testEnv.state.rewindABlock()                                                            // Rewind to parent

	// Make fork2 the chosen fork.
	err = testEnv.state.AcceptBlock(*fork2a)
	if err != nil {
		testEnv.t.Fatal(err)
	}
	// Verify that fork2a is the current block.
	if testEnv.state.CurrentBlock != fork2a.ID() {
		testEnv.t.Fatal("fork2 not accepted as farthest node.")
	}

	// Add fork1b (rewinding does not remove a block from the
	// state) to the state and see if the forking happens.
	err = testEnv.state.AcceptBlock(*fork1b)
	if err != nil {
		testEnv.t.Fatal(err)
	}
	// Verify that fork1b is the current block.
	if testEnv.state.CurrentBlock != fork1b.ID() {
		testEnv.t.Fatal("switching to a heavier chain did not appear to work.")
	}

	// Fork from the current chain to a different chain, but be required to
	// double back from validation problems.
}

// successContractTests() creates a contract and does successful proofs on the
// contract, until successful termination is achieved.
func successContractTests(testEnv *testingEnvironment) {
	// Create a contract with funds from wallet 0 and wallet 1.
	fcp := FileContractParameters{
		Transaction: Transaction{
			FileContracts: []FileContract{
				FileContract{
					ContractFund:       5,
					Start:              testEnv.state.Height() + 1,
					End:                testEnv.state.Height() + 2,
					ChallengeFrequency: 1,
					Tolerance:          1,
					ValidProofPayout:   1,
					ValidProofAddress:  testEnv.wallets[0].SpendConditions.CoinAddress(),
					MissedProofPayout:  3,
					MissedProofAddress: testEnv.wallets[1].SpendConditions.CoinAddress(),
				},
			},
		},
		ClientContribution: 3,
	}

	// Add funds to the contract from each wallet.
	testEnv.wallets[0].Scan(testEnv.state)
	err := testEnv.wallets[0].FundTransaction(fcp.ClientContribution, &fcp.Transaction)
	if err != nil {
		testEnv.t.Fatal(err)
	}
	testEnv.wallets[1].Scan(testEnv.state)
	err = testEnv.wallets[1].FundTransaction(fcp.Transaction.FileContracts[0].ContractFund-fcp.ClientContribution, &fcp.Transaction)
	if err != nil {
		testEnv.t.Fatal(err)
	}

	// Sign the transaction using each wallet.
	testEnv.wallets[0].SignTransaction(&fcp.Transaction)
	testEnv.wallets[1].SignTransaction(&fcp.Transaction)

	// Get the transaction into a block and get the block into the state.
	err = testEnv.state.AcceptTransaction(fcp.Transaction)
	if err != nil {
		testEnv.t.Fatal(err)
	}
	block := testEnv.state.GenerateBlock(testEnv.wallets[1].SpendConditions.CoinAddress())
	err = testEnv.state.AcceptBlock(*block)
	if err != nil {
		testEnv.t.Fatal(err)
	}

	// Check that the correct OpenContracts object has been created, and
	// contains the expected values.
	contractID := fcp.Transaction.FileContractID(0)
	openContract, exists := testEnv.state.OpenContracts[contractID]
	if !exists {
		testEnv.t.Fatal("open contract not found")
	}
	if openContract.ContractID != contractID {
		testEnv.t.Fatal("open contract has wrong contract id")
	}
	if openContract.FundsRemaining != fcp.Transaction.FileContracts[0].ContractFund {
		testEnv.t.Fatal("open contract has wrong listed number of remaining funds.")
	}
	if openContract.Failures != 0 {
		testEnv.t.Fatal("open contract has non-zero number of failures")
	}
	if !openContract.WindowSatisfied {
		testEnv.t.Fatal("open contract has started with WindowSatisfied = false")
	}
}

// For now, this is really just a catch-all test. I'm not really sure how to
// modularize the various components =/
func TestBlockBuilding(t *testing.T) {
	// Initialize the testing evironment.
	testEnv := createEnvironment(t)

	// Add an empty block to the testing environment.
	addEmptyBlock(testEnv)

	// Add a block with a transaction and see if the proper outputs are
	// created, checking the values and spendhashes in the state.

	// Create a few new wallets and send coins to each in a block.
	transactionPoolTests(testEnv)

	// Create a test that submits and removes transactions with multiple
	// inputs and outputs.

	// Test rewinding a block.

	// Probe the block forking code.
	blockForkingTests(testEnv)

	// Probe the difficulty adjustment code.

	// Test adding a contract to the blockchain where proofs are submitted
	// until succesful termination.
	successContractTests(testEnv)
}
