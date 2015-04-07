package transactionpool

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
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
	if err != ErrUnrecognizedSiacoinInput {
		tpt.t.Error(err)
	}
}

// testFalseSiacoinSpend spends a nonexistent siacoin output in a transaction
// and checks that the transaction is rejected.
func (tpt *tpoolTester) testFalseSiacoinSpend() {
	txn := tpt.emptyUnlockTransaction()
	txn.SiacoinInputs[0].ParentID[0] -= 1
	err := tpt.tpool.AcceptTransaction(txn)
	if err != ErrUnrecognizedSiacoinInput {
		tpt.t.Error(err)
	}
}

// testBadSiacoinUnlock attempts to submit a transaction where the unlock
// conditions + signature are valid but they don't match the unlock hash of the
// transaction.
func (tpt *tpoolTester) testBadSiacoinUnlock() {
	// The empty unlock signature will still be valid, as the new conditions
	// merely enforce a timelock. The unlock hash however will not match.
	txn := tpt.emptyUnlockTransaction()
	altConditions := types.UnlockConditions{
		Timelock: 1,
	}
	txn.SiacoinInputs[0].UnlockConditions = altConditions
	err := tpt.tpool.AcceptTransaction(txn)
	if err != ErrBadUnlockConditions {
		tpt.t.Error(err)
	}
}

// testOverspend submits a transaction that has more outputs than inputs. This
// submission should be rejected.
func (tpt *tpoolTester) testOverspend() {
	txn := tpt.emptyUnlockTransaction()
	txn.MinerFees = append(txn.MinerFees, types.NewCurrency64(1))
	err := tpt.tpool.AcceptTransaction(txn)
	if err != ErrSiacoinOverspend {
		tpt.t.Error(err)
	}
}

// TestAddConflictingSiacoinTransactionToPool creates a tpoolTester and uses it
// to call addConflictingSiacoinTransactionToPool.
func TestAddConflictingSiacoinTransaction(t *testing.T) {
	tpt := newTpoolTester("TestAddConflictingSiacoinTransaction", t)
	tpt.addConflictingSiacoinTransaction()
}

// TestFalseSiacoinSpend creates a tpoolTester and uses it to call smaller
// tests that probe the siacoin verification rules.
func TestFalseSiacoinFeatures(t *testing.T) {
	tpt := newTpoolTester("TestFalseSiacoinFeatures", t)
	tpt.testFalseSiacoinSpend()
	tpt.testBadSiacoinUnlock()
	tpt.testOverspend()
}
