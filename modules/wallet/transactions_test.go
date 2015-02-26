package wallet

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
)

// testFundTransaction funds and completes a transaction using the
// build-your-own transaction functions, checking that a no-refund transaction
// is created that is valid.
func (wt *WalletTester) testFundTransaction() {
	// Get a coin address for the wallet and fund the wallet using money from
	// the
	addr, _, err := wt.CoinAddress()
	if err != nil {
		wt.Fatal(err)
	}
	siacoinOutput, value := wt.FindSpendableSiacoinInput()
	walletFunderTxn := wt.AddSiacoinInputToTransaction(consensus.Transaction{}, siacoinOutput)
	walletFunderOutput := consensus.SiacoinOutput{
		Value:      value,
		UnlockHash: addr,
	}
	walletFunderTxn.SiacoinOutputs = append(walletFunderTxn.SiacoinOutputs, walletFunderOutput)
	block := wt.MineCurrentBlock([]consensus.Transaction{walletFunderTxn})
	err = wt.State.AcceptBlock(block)
	if err != nil {
		wt.Fatal(err)
	}
	wt.update()

	// Build a transaction that intentionally needs a refund.
	id, err := wt.RegisterTransaction(consensus.Transaction{})
	if err != nil {
		wt.Fatal(err)
	}
	_, err = wt.FundTransaction(id, value.Sub(consensus.NewCurrency64(1)))
	if err != nil {
		wt.Fatal(err)
	}
	_, _, err = wt.AddMinerFee(id, value.Sub(consensus.NewCurrency64(1)))
	if err != nil {
		wt.Fatal(err)
	}
	t, err := wt.SignTransaction(id, true)
	if err != nil {
		wt.Fatal(err)
	}
	err = wt.tpool.AcceptTransaction(t)
	if err != nil {
		wt.Fatal(err)
	}
	wt.update()

	// Check that the length of the created transaction is 1 siacoin, and that
	// the unconfirmed balance of the wallet is 1.
	if len(t.SiacoinOutputs) != 0 {
		wt.Error("more than zero siacoin outputs created in custom transaction")
	}
	if wt.Balance(true).Cmp(consensus.NewCurrency64(1)) != 0 {
		wt.Error(wt.Balance(true).MarshalJSON())
		wt.Error("wallet balance not reporting at one?")
	}

	// Dump the transaction pool into a block and see that the balance still
	// registers correctly.
	txns, err := wt.tpool.TransactionSet()
	if err != nil {
		wt.Error(err)
	}
	block = wt.MineCurrentBlock(txns)
	err = wt.State.AcceptBlock(block)
	if err != nil {
		wt.Error(err)
	}

	// Check that the length of the created transaction is 1 siacoin, and that
	// the unconfirmed balance of the wallet is 1.
	if len(t.SiacoinOutputs) != 0 {
		wt.Error("more than zero siacoin outputs created in custom transaction")
	}
	if wt.Balance(true).Cmp(consensus.NewCurrency64(1)) != 0 {
		wt.Error(wt.Balance(true).MarshalJSON())
		wt.Error("wallet balance not reporting at one?")
	}
}

// TestFundTransaction creates a wallet tester and uses it to call
// testFundTransaction.
func TestFundTransaction(t *testing.T) {
	wt := NewWalletTester(t)
	wt.testFundTransaction()
}
