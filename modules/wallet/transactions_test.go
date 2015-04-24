package wallet

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// testFundTransaction funds and completes a transaction using the
// build-your-own transaction functions, checking that a no-refund transaction
// is created that is valid.
func (wt *walletTester) testFundTransaction() {
	// Build a transaction that intentionally needs a refund.
	id, err := wt.wallet.RegisterTransaction(types.Transaction{})
	fund := wt.wallet.Balance(false).Sub(types.NewCurrency64(1))
	if err != nil {
		wt.t.Fatal(err)
	}
	_, err = wt.wallet.FundTransaction(id, fund)
	if err != nil {
		wt.t.Fatal(err)
	}
	wt.tpUpdateWait()
	_, _, err = wt.wallet.AddMinerFee(id, fund)
	if err != nil {
		wt.t.Fatal(err)
	}
	t, err := wt.wallet.SignTransaction(id, true)
	if err != nil {
		wt.t.Fatal(err)
	}
	err = wt.tpool.AcceptTransaction(t)
	if err != nil {
		wt.t.Fatal(err)
	}
	wt.tpUpdateWait()

	// Check that the length of the created transaction is 1 siacoin, and that
	// the unconfirmed balance of the wallet is 1.
	if len(t.SiacoinOutputs) != 0 {
		wt.t.Error("more than zero siacoin outputs created in custom transaction")
	}
	if wt.wallet.Balance(true).Cmp(types.NewCurrency64(1)) != 0 {
		wt.t.Error(wt.wallet.Balance(true))
		wt.t.Error("wallet balance not reporting at one?")
	}

	// Dump the transaction pool into a block and see that the balance still
	// registers correctly.
	_, _, err = wt.miner.FindBlock()
	if err != nil {
		wt.t.Fatal(err)
	}

	// Check that the length of the created transaction is 1 siacoin, and that
	// the unconfirmed balance of the wallet is 1.
	if len(t.SiacoinOutputs) != 0 {
		wt.t.Error("more than zero siacoin outputs created in custom transaction")
	}
	if wt.wallet.Balance(true).Cmp(types.NewCurrency64(1)) != 0 {
		wt.t.Error(wt.wallet.Balance(true))
		wt.t.Error("wallet balance not reporting at one?")
	}
}

// TestFundTransaction creates a wallet tester and uses it to call
// testFundTransaction.
func TestFundTransaction(t *testing.T) {
	wt := NewWalletTester("TestFundTransaction", t)
	wt.testFundTransaction()
}
