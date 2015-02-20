package transactionpool

import (
	"testing"
)

// testTransactionDumping calls TransactionSet and puts the transactions in a
// block that gets submitted to the state. If there is an error, the
// transaction set is known to be invalid.
func (tpt *TpoolTester) testTransactionDumping() {
	// Get the transaction set.
	tset, err := tpt.TransactionSet()
	if err != nil {
		tpt.Error(err)
	}

	// Add the transaction set to a block and check that it is valid in the
	// state by adding it to the state.
	b := tpt.MineCurrentBlock(tset)
	err = tpt.State.AcceptBlock(b)
	if err != nil {
		tpt.Error(err)
	}
}

// testSiacoinTransactionDump adds a handful of siacoin transactions to the
// transaction pool and then runs testTransactionDumping to see that the pool
// set follows the rules of the blockchain.
func (tpt *TpoolTester) testSiacoinTransactionDump() {
	tpt.addDependentSiacoinTransactionToPool()
	tpt.testTransactionDumping()
}

// testUnconfirmedSiacoinOutputDiffs adds some unconfirmed transactions to the
// transaction pool and then checks the diffs. Then a block is put into the
// state with the transactions and the diffs are checked again.
func (tpt *TpoolTester) testUnconfirmedSiacoinOutputDiffs() {
	tpt.addDependentSiacoinTransactionToPool()
	diffs := tpt.UnconfirmedSiacoinOutputDiffs()
	if len(diffs) != 3 {
		tpt.Error("wrong number of diffs")
	}
}

// TestSiacoinTransactionDump creates a TpoolTester and uses it to call
// testSiacoinTransactionDump.
func TestSiacoinTransactionDump(t *testing.T) {
	tpt := CreateTpoolTester(t)
	tpt.testSiacoinTransactionDump()
}

// TestUnconfirmedSiacoinOutputDiffs creates a TpoolTester and uses it to call
// testUnconfirmedSiacoinOutputDiffs.
func TestUnconfirmedSiacoinOutputDiffs(t *testing.T) {
	tpt := CreateTpoolTester(t)
	tpt.testUnconfirmedSiacoinOutputDiffs()
}
