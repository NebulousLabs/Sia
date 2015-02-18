package wallet

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
)

// TestCoinAddress fetches a coin address from the wallet and then spends an
// output to the coin address to verify that the wallet is correctly
// recognizing coins sent to itself.
func TestCoinAddress(t *testing.T) {
	// Create a testing environment and then a wallet.
	a := consensus.NewTestingEnvironment(t)
	tpool, err := transactionpool.New(a.State)
	if err != nil {
		t.Fatal(err)
	}
	w, err := New(a.State, tpool, "")
	if err != nil {
		t.Fatal(err)
	}

	// Get an address.
	walletAddress, _, err := w.CoinAddress()
	if err != nil {
		t.Fatal(err)
	}

	// Send coins to the address, in a mined block.
	siacoinInput, value := a.FindSpendableSiacoinInput()
	txn := a.AddSiacoinInputToTransaction(consensus.Transaction{}, siacoinInput)
	txn.SiacoinOutputs = append(txn.SiacoinOutputs, consensus.SiacoinOutput{
		Value:      value,
		UnlockHash: walletAddress,
	})
	block, err := a.MineCurrentBlock([]consensus.Transaction{txn})
	if err != nil {
		t.Fatal(err)
	}
	err = a.State.AcceptBlock(block)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the wallet sees the coins.
	if w.Balance(false).Cmp(consensus.ZeroCurrency) == 0 {
		t.Error("wallet didn't get the coins sent to it.")
	}
}
