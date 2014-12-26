package sia

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
)

// testSendToSelf does a send from the wallet to itself, and checks that all of
// the balance reporting at each step makes sense, and then checks that all of
// the coins are still sendable.
func testSendToSelf(t *testing.T, e *Core) {
	if e.wallet.Balance(false) == 0 {
		t.Error("e.wallet is empty.")
		return
	}
	originalBalance := e.wallet.Balance(false)

	// Get a new coin address from the wallet and send the coins to yourself.
	dest, err := e.wallet.CoinAddress()
	if err != nil {
		t.Error(err)
		return
	}
	txn, err := e.SpendCoins(e.wallet.Balance(false)-10, dest)
	if err != nil {
		t.Error(err)
		return
	}

	// Process the transaction and check the balance, which should now be 0.
	err = e.processTransaction(txn)
	if err != nil {
		t.Error(err)
	}
	if e.wallet.Balance(false) != 0 {
		t.Error("Expecting a balance of 0, got", e.wallet.Balance(false))
	}

	// Mine the block and check the balance, which should now be
	// originalBalance + Coinbase.
	mineSingleBlock(t, e)
	if e.wallet.Balance(false) != originalBalance+consensus.CalculateCoinbase(e.Height()) {
		t.Errorf("Expecting a balance of %v, got %v", originalBalance+consensus.CalculateCoinbase(e.Height()), e.wallet.Balance(false))
	}
	if e.wallet.Balance(false) != e.wallet.Balance(true) {
		t.Errorf("Expecting balance and full balance to be equal, but instead they are false: %v, full: %v", e.wallet.Balance(false), e.wallet.Balance(true))
	}
}

// testWalletInfo calles wallet.Info to see if an error is thrown. Also make sure
// there is no deadlock.
func testWalletInfo(t *testing.T, c *Core) {
	_, err := c.WalletInfo()
	if err != nil {
		t.Error(err)
	}
}
