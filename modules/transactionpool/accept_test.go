package transactionpool

import (
	"testing"
)

// TestIntegrationAcceptTransactionSet probes the AcceptTransactionSet method
// of the transaction pool.
func TestIntegrationAcceptTransactionSet(t *testing.T) {
	// Create a transaction pool tester.
	tpt, err := createTpoolTester("TestIntegrationAcceptTransactionSet")
	if err != nil {
		t.Fatal(err)
	}

	// Check that the transaction pool is empty.
	if len(tpt.transactionSets) != 0 {
		t.Error("transaction pool is not empty")
	}

	// Create a valid transaction set using the wallet.
	_, err := tpt.wallet.SendCoins(types.NewCurrency64(100), types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tpt.transactionSets) != 1 {
		t.Error("sending coins did not increase the transaction sets by 1")
	}
}
