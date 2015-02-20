package wallet

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
)

// TestCoinAddress fetches a coin address from the wallet and then spends an
// output to the coin address to verify that the wallet is correctly
// recognizing coins sent to itself.
func (wt *walletTester) testCoinAddress() {
	// Get an address.
	walletAddress, _, err := wt.CoinAddress()
	if err != nil {
		wt.Fatal(err)
	}

	// Send coins to the address, in a mined block.
	siacoinInput, value := wt.FindSpendableSiacoinInput()
	txn := wt.AddSiacoinInputToTransaction(consensus.Transaction{}, siacoinInput)
	txn.SiacoinOutputs = append(txn.SiacoinOutputs, consensus.SiacoinOutput{
		Value:      value,
		UnlockHash: walletAddress,
	})
	block := wt.MineCurrentBlock([]consensus.Transaction{txn})
	err = wt.State.AcceptBlock(block)
	if err != nil {
		wt.Fatal(err)
	}

	// Check that the wallet sees the coins.
	if wt.Balance(false).Cmp(consensus.ZeroCurrency) == 0 {
		wt.Error("wallet didn't get the coins sent to it.")
	}
}

// TestCoinAddress creates a new wallet tester and uses it to call
// testCoinAddress.
func TestCoinAddress(t *testing.T) {
	wt := newWalletTester(t)
	wt.testCoinAddress()
}
