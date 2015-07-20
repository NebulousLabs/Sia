package transactionpool

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
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
	if len(tpt.tpool.transactionSets) != 0 {
		t.Error("transaction pool is not empty")
	}

	// Create a valid transaction set using the wallet.
	txns, err := tpt.wallet.SendCoins(types.NewCurrency64(100), types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tpt.tpool.transactionSets) != 1 {
		t.Error("sending coins did not increase the transaction sets by 1")
	}

	// Submit the transaction set again to trigger a duplication error.
	err = tpt.tpool.AcceptTransactionSet(txns)
	if err != modules.ErrDuplicateTransactionSet {
		t.Error(err)
	}

	// Mine a block and check that the transaction pool gets emptied.
	block, _ := tpt.miner.FindBlock()
	err = tpt.cs.AcceptBlock(block)
	if err != nil {
		t.Fatal(err)
	}
	if len(tpt.tpool.transactionSets) != 0 {
		t.Error("transaction pool was not emptied after mining a block")
	}

	// Try to resubmit the transaction set to verify
	err = tpt.tpool.AcceptTransactionSet(txns)
	if err == nil {
		t.Error("transaction set was apparently not mined into a block")
	}
}

// TestIntegrationCheckMinerFees probes the checkMinerFees method of the
// transaction pool.
func TestIntegrationCheckMinerFees(t *testing.T) {
	// Create a transaction pool tester.
	tpt, err := creatTpoolTester("TestIntegrationCheckMinerFees")
	if err != nil {
		t.Fatal(err)
	}

	// Fill the transaction pool to the fee limit.
	for i := 0; i < TransactionPoolSizeForFee / 10e3; i++ {
		arbData := make([]byte, 10e3)
		txn := types.Transaction{ArbitraryData: [][]byte{arbData}}
		tpt.tpool.AcceptTransactionSet([]types.Transaction{txn})
	}
	// t.Error(tpt.tpool.I//
}
