package transactionpool

import (
	"testing"
)

// testTransactionDumping calls TransactionSet and puts the transactions in a
// block that gets submitted to the state. If there is an error, the
// transaction set is known to be invalid.
func (tpt *tpoolTester) testSiacoinTransactionDump() {
	tlen := len(tpt.tpool.TransactionSet())

	tpt.addDependentSiacoinTransactionToPool()
	if tlen >= len(tpt.tpool.TransactionSet()) {
		tpt.t.Error("wrong number of transactions in transaction dump, expecting mor than", tlen, "got", len(tpt.tpool.TransactionSet()))
	}

	// Add the transaction set to a block and check that it is valid in the
	// state by adding it to the state.
	_, _, err := tpt.miner.FindBlock()
	if err != nil {
		tpt.t.Error(err)
	}
	tpt.csUpdateWait()
	if len(tpt.tpool.TransactionSet()) != 0 {
		tpt.t.Error("Transaction set should be empty after mining a block, instead is size", len(tpt.tpool.TransactionSet()))
	}
}

// TestSiacoinTransactionDump creates a tpoolTester and uses it to call
// testSiacoinTransactionDump.
func TestSiacoinTransactionDump(t *testing.T) {
	tpt := newTpoolTester("TestSiacoinTransactionDump", t)
	tpt.testSiacoinTransactionDump()
}
