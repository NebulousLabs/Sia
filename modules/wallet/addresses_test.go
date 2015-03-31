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
	walletAddress, _, err := wt.wallet.CoinAddress()
	if err != nil {
		wt.t.Fatal(err)
	}

	// Send all of the wallets coins to itself.
	wt.spendCoins(wt.wallet.Balance(false), walletAddress)

	// Check that the wallet sees the coins.
	if wt.wallet.Balance(false).Cmp(consensus.ZeroCurrency) == 0 {
		wt.t.Error("wallet didn't get the coins sent to it.")
	}
}

// TestCoinAddress creates a new wallet tester and uses it to call
// testCoinAddress.
func TestCoinAddress(t *testing.T) {
	wt := NewWalletTester("TestCoinAddress", t)
	wt.testCoinAddress()
}
