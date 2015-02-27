package wallet

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
)

// testUnconfirmedTransaction has money sent to itself through an unconfirmed
// transaction in the transaction pool, and checks that 'update' adds the
// balance to the wallet's total funds.
func (wt *WalletTester) testUnconfirmedTransaction() {
	// Get the initial balance, then get an address that can receive coins.
	initialBal := wt.Balance(false)
	address, _, err := wt.CoinAddress()
	if err != nil {
		wt.Fatal(err)
	}

	// Get a transaction with coins sent to the wallet's address.
	input, value := wt.FindSpendableSiacoinInput()
	txn := wt.AddSiacoinInputToTransaction(consensus.Transaction{}, input)
	output := consensus.SiacoinOutput{
		Value:      value,
		UnlockHash: address,
	}
	txn.SiacoinOutputs = append(txn.SiacoinOutputs, output)
	err = wt.tpool.AcceptTransaction(txn)
	if err != nil {
		wt.Fatal(err)
	}

	// Update the wallet and check that the balance has increased by value.
	wt.update()
	expectedBal := initialBal.Add(value)
	if expectedBal.Cmp(wt.Balance(false)) != 0 {
		wt.Error("unexpected balance after adding unconfirmed transaction")
	}
}

// TestUnconfirmedTransaction creates a wallet tester and uses it to call
// testUnconfirmedTransaction.
func TestUnconfirmedTransaction(t *testing.T) {
	wt := NewWalletTester(t)
	wt.testUnconfirmedTransaction()
}
