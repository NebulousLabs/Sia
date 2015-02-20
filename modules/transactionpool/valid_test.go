package transactionpool

import (
	"testing"
)

// addConflictingSiacoinTransactionToPool creates a valid transaction, adds it
// to the pool, and then tries to submit a transaction that uses the same
// outputs and checks that the double-spend attempt is caught by the pool.
func (tpt *TpoolTester) addConflictingSiacoinTransactionToPool() {
	// Put a siacoin transaction in to the transaciton pool.
	txn := tpt.addSiacoinTransactionToPool()

	// Try to add the same transaction to the transaction pool.
	err := tpt.AcceptTransaction(txn)
	if err != ErrDoubleSpend {
		tpt.Fatal(err)
	}
}

// TestAddConflictingSiacoinTransactionToPool creates a tpoolTest and uses it
// to call addConflictingSiacoinTransactionToPool.
func TestAddConflictingSiacoinTransactionToPool(t *testing.T) {
	tpt := CreateTpoolTester(t)
	tpt.addConflictingSiacoinTransactionToPool()
}
