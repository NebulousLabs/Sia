package transactionpool

import (
	"testing"
)

// addConflictingSiacoinTransactionToPool creates a valid transaction, adds it
// to the pool, and then tries to submit a transaction that uses the same
// outputs and checks that the double-spend attempt is caught by the pool.
func (tpt *tpoolTester) addConflictingSiacoinTransaction() {
	txn := tpt.emptyUnlockTransaction()

	// Try to add a double spend transaction to the pool.
	err := tpt.tpool.AcceptTransaction(txn)
	if err != nil {
		tpt.t.Error(err)
	}
	txn.ArbitraryData = append(txn.ArbitraryData, "NonSia: this stops the transaction from being a duplicate")
	err = tpt.tpool.AcceptTransaction(txn)
	if err != ErrDoubleSpend {
		tpt.t.Error(err)
	}
}

// TestAddConflictingSiacoinTransactionToPool creates a tpoolTest and uses it
// to call addConflictingSiacoinTransactionToPool.
func TestAddConflictingSiacoinTransaction(t *testing.T) {
	tpt := newTpoolTester("Transaction Pool - TestAddConflictingSiacoinTransaction", t)
	tpt.addConflictingSiacoinTransaction()
}
