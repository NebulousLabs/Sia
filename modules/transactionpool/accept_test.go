package transactionpool

import (
	"crypto/rand"
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
	if len(tpt.tpool.TransactionList()) != 0 {
		t.Error("transaction pool was not emptied after mining a block")
	}

	// Try to resubmit the transaction set to verify
	err = tpt.tpool.AcceptTransactionSet(txns)
	if err == nil {
		t.Error("transaction set was supposed to be rejected")
	}
}

// TestIntegrationCheckMinerFees probes the checkMinerFees method of the
// transaction pool.
func TestIntegrationCheckMinerFees(t *testing.T) {
	// Create a transaction pool tester.
	tpt, err := createTpoolTester("TestIntegrationCheckMinerFees")
	if err != nil {
		t.Fatal(err)
	}

	// Fill the transaction pool to the fee limit.
	for i := 0; i < TransactionPoolSizeForFee/10e3; i++ {
		arbData := make([]byte, 10e3)
		copy(arbData, modules.PrefixNonSia[:])
		_, err = rand.Read(arbData[100:116]) // prevents collisions with other transacitons in the loop.
		if err != nil {
			t.Fatal(err)
		}
		txn := types.Transaction{ArbitraryData: [][]byte{arbData}}
		err := tpt.tpool.AcceptTransactionSet([]types.Transaction{txn})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Add another transaction, this one should fail for having too few fees.
	err = tpt.tpool.AcceptTransactionSet([]types.Transaction{{}})
	if err != ErrLowMinerFees {
		t.Error(err)
	}

	// Add a transaction that has sufficient fees.
	_, err = tpt.wallet.SendCoins(types.NewCurrency64(100), types.UnlockHash{})
	if err != nil {
		t.Error(err)
	}

	// TODO: fill the pool up all the way and try again.
}
