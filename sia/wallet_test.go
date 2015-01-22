package sia

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
)

// testSendToSelf does a send from the wallet to itself, and checks that all of
// the balance reporting at each step makes sense, and then checks that all of
// the coins are still sendable.
func testSendToSelf(t *testing.T, c *Core) {
	if c.wallet.Balance(false) == 0 {
		t.Error("c.wallet is empty.")
		return
	}
	originalBalance := c.wallet.Balance(false)

	// Get a new coin address from the wallet and send the coins to yourself.
	dest, _, err := c.wallet.CoinAddress()
	if err != nil {
		t.Error(err)
		return
	}
	_, err = c.wallet.SpendCoins(c.wallet.Balance(false)-10, dest)
	if err != nil {
		t.Error(err)
		return
	}

	// Process the transaction and check the balance, which should now be 0.
	//
	// TODO: This error checking is hacky, instead should use some sort of
	// synchronization technique.
	if c.wallet.Balance(false) != 0 {
		t.Error("Expecting a balance of 0, got", c.wallet.Balance(false))
	}

	// Mine the block and check the balance, which should now be
	// originalBalance + Coinbase.
	mineSingleBlock(t, c)
	if c.wallet.Balance(false) != originalBalance+consensus.CalculateCoinbase(c.state.Height()) {
		t.Errorf("Expecting a balance of %v, got %v", originalBalance+consensus.CalculateCoinbase(c.state.Height()), c.wallet.Balance(false))
	}
	if c.wallet.Balance(false) != c.wallet.Balance(true) {
		t.Errorf("Expecting balance and full balance to be equal, but instead they are false: %v, full: %v", c.wallet.Balance(false), c.wallet.Balance(true))
	}
}

// testWalletInfo calles wallet.Info to see if an error is thrown. Also make sure
// there is no deadlock.
func testWalletInfo(t *testing.T, c *Core) {
	_, err := c.wallet.WalletInfo()
	if err != nil {
		t.Error(err)
	}
}
