package siacore

/*
import (
	"time"
)

// testTransactionSending sends a transaction from one host to another,
// verifying that the transaction pool is updating correctly in the process and
// also doing some state checks.
func testTransactionSending(te *testEnv) {
	// Check that e0 and e1 both have mining disabled.
	if te.e0.Mining() || te.e1.Mining() {
		te.t.Error("cannot do transaction sending tests if the testEnv miners are active.")
		return
	}

	// Check that e0 and e1 both have an empty transaction list.
	if len(te.e0.state.TransactionList()) != 0 || len(te.e1.state.TransactionList()) != 0 {
		te.t.Error("Transaction lists are not empty.")
		return
	}

	// Create a third environment to do the mining, so that we can be certain
	// about the balances of the first two environments, and wait so CatchUp()
	// can complete.
	miningEnv, err := CreateEnvironment(9990, 9982, false)
	if err != nil {
		te.t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)

	// Get the initial balances of each environment.
	e0Bal := te.e0.WalletBalance()
	e1Bal := te.e1.WalletBalance()
	if e0Bal < 300 {
		te.t.Error("e0 does not have a high enough balance to complete transaction sending tests.")
		return
	}

	// Send coins from e0 to e1
	_, err = te.e0.SpendCoins(250, 25, te.e1.CoinAddress())
	if err != nil {
		te.t.Error(err)
	}

	// Check that the 3 environments are all on the same page in terms of
	// blockchain and transaction pool.
	if miningEnv.Height() != te.e0.Height() {
		te.t.Error("miningEnv height does not equal e0 height.")
		return
	}
	if te.e0.Height() != te.e1.Height() {
		te.t.Error("e0 height does not equal e1 height.")
		return
	}

	// Check that the transaction pools in all three states contain the
	// transaction, after giving a second for the information to propagate.
	time.Sleep(300 * time.Millisecond)
	if len(te.e0.TransactionList()) != 1 || len(te.e1.TransactionList()) != 1 || len(miningEnv.TransactionList()) != 1 {
		te.t.Error("transaction has not properly propagated through the transaction pool.", len(te.e0.TransactionList()), ":", len(te.e1.TransactionList()), ":", len(miningEnv.TransactionList()))
		return
	}

	// Mine a block to have the transaction confirmed, and give a second for the block to propagate.
	miningEnv.mineSingleBlock()
	time.Sleep(300 * time.Millisecond)

	// Check that the balances have adjusted accordingly.
	if te.e0.WalletBalance() != e0Bal-275 || te.e1.WalletBalance() != e1Bal+250 {
		te.t.Error("wallet balances did not update after mining transaction")
		return
	}
}

// testLargeTransactions creates a transaction out of many outputs and verifies
// that things still run smoothly.
func testLargeTransactions(te *testEnv) {
	// Check that no mining is happening.
	if te.e0.Mining() || te.e1.Mining() {
		te.t.Error("cannot do testLargeTransactions while an environment is mining!")
		return
	}

	// Check that e0 has sufficient balance.
	if te.e0.WalletBalance() < 2500 {
		te.t.Error("e0 has insufficient balance to complete testLargeTransactions")
		return
	}

	// Test has been put on hold until you can spend new outputs in the same
	// block that they are made.
		// Get e1 up to having at least 50 inputs.
		te.e1.wallet.Scan()
		for i := len(te.e1.wallet.OwnedOutputs); i <= 5; i++ {
			_, err = te.e0.SpendCoins(50, 0, te.e1.CoinAddress())
			if err != nil {
				te.t.Error(err)
				return
			}
		}

		// Mine a block to have the transactions accepted, and wait for the block
		// to propagate.
		te.e0.mineSingleBlock()
		time.Sleep(600 * time.Millisecond)

		// Check that all of the outputs have been received.
		te.e1.wallet.Scan()
		if len(te.e1.wallet.OwnedOutputs) < 5 {
			te.t.Error("e1 does not have enough outputs to complete test.")
			println(len(te.e1.wallet.OwnedOutputs))
			return
		}
}
*/
