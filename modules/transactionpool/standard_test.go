package transactionpool

import (
	"crypto/rand"
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// Try to add a transaction that is too large to the transaction pool.
func TestLargeTransaction(t *testing.T) {
	tpt := newTpoolTester("TestLargeTransaction", t)

	// Create a transaction that's larger than the size limit.
	largeArbitraryData := make([]byte, TransactionSizeLimit)
	rand.Read(largeArbitraryData)
	acceptableData := "NonSia" + string(largeArbitraryData)
	txn := types.Transaction{
		ArbitraryData: []string{acceptableData},
	}

	// Check IsStandard.
	err := tpt.tpool.IsStandardTransaction(txn)
	if err != errLargeTransaction {
		t.Error("expecting errLargeTransaction, got:", err)
	}

	// Check that transaction is rejected when calling 'accept'.
	err = tpt.tpool.AcceptTransaction(txn)
	if err != errLargeTransaction {
		t.Error("expecting errLargeTransaction, got:", err)
	}
	if len(tpt.tpool.TransactionSet()) != 0 {
		t.Error("tpool is not empty after accepting a bad transaction")
	}
}
