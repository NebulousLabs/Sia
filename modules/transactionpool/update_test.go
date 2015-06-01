package transactionpool

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// testUpdateTransactionRemoval checks that when a transaction moves from the
// unconfirmed set into the confirmed set, the transaction gets correctly
// removed from the unconfirmed set.
func (tpt *tpoolTester) testUpdateTransactionRemoval() {
	// Create some unconfirmed transactions.
	tpt.addDependentSiacoinTransactionToPool()
	if len(tpt.tpool.TransactionSet()) == 0 {
		tpt.t.Error("tset should have some transacitons")
	}

	// Mine a block to put the transactions into the confirmed set.
	_, _, err := tpt.miner.FindBlock()
	if err != nil {
		tpt.t.Fatal(err)
	}
	tpt.csUpdateWait()

	// Check that the transactions have been removed from the unconfirmed set.
	if len(tpt.tpool.TransactionSet()) != 0 {
		tpt.t.Error("unconfirmed transaction set is not empty, len", len(tpt.tpool.TransactionSet()))
	}
}

// testDataTransactions checks transactions that are data-only, and makes sure
// that the data is put into the blockchain only a single time.
func (tpt *tpoolTester) testDataTransactions() {
	// Make a data transaction and put it into the blockchain.
	txn := types.Transaction{
		ArbitraryData: []string{"NonSiadata"},
	}
	err := tpt.tpool.AcceptTransaction(txn)
	if err != nil {
		tpt.t.Fatal(err)
	}
	tpt.tpUpdateWait()
	b, _, err := tpt.miner.FindBlock()
	if err != nil {
		tpt.t.Fatal(err)
	}
	if len(b.Transactions) != 2 {
		tpt.t.Error(len(b.Transactions))
		tpt.t.Fatal("only expecting 2 transactions in the test block")
	}
	tpt.csUpdateWait()

	// Mine a second block, this block should not have the data transaction.
	b, _, err = tpt.miner.FindBlock()
	if err != nil {
		tpt.t.Fatal(err)
	}
	if len(b.Transactions) != 1 {
		tpt.t.Fatal("Block should contain only the uniqueness transaction after mining a data transaction")
	}
	tpt.csUpdateWait()

}

// testBlockConflicts adds a transaction to the unconfirmed set, and then adds
// a conflicting transaction to the confirmed set, checking that the conflict
// is properly handled by the pool.
func (tpt *tpoolTester) testBlockConflicts() {
	// Put two transactions, a parent and a dependent, into the transaction
	// pool. Then create a transaction that is in conflict with the parent.
	parent := tpt.emptyUnlockTransaction()
	dependent := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{
			types.SiacoinInput{
				ParentID: parent.SiacoinOutputID(0),
			},
		},
		MinerFees: []types.Currency{
			parent.SiacoinOutputs[0].Value,
		},
	}
	err := tpt.tpool.AcceptTransaction(parent)
	if err != nil {
		tpt.t.Fatal(err)
	}
	tpt.tpUpdateWait()
	err = tpt.tpool.AcceptTransaction(dependent)
	if err != nil {
		tpt.t.Fatal(err)
	}
	tpt.tpUpdateWait()

	// Create a transaction that is in conflict with the parent.
	parentValue := parent.SiacoinOutputSum()
	conflict := types.Transaction{
		SiacoinInputs: parent.SiacoinInputs,
		MinerFees: []types.Currency{
			parentValue,
		},
	}

	// Mine a block to put the conflict into the confirmed set. 'parent' has
	// dependencies of it's own, and 'conflict' has the same dependencies as
	// 'parent'. So the block we mine needs to include all of the dependencies
	// without including 'parent' or 'dependent'.
	tset := tpt.tpool.TransactionSet()
	tset = tset[:len(tset)-2]     // strip 'parent' and 'dependent'
	tset = append(tset, conflict) // add 'conflict'
	target := tpt.cs.CurrentTarget()
	block := types.Block{
		ParentID:  tpt.cs.CurrentBlock().ID(),
		Timestamp: types.Timestamp(time.Now().Unix()),
		MinerPayouts: []types.SiacoinOutput{
			types.SiacoinOutput{Value: parentValue.Add(types.CalculateCoinbase(tpt.cs.Height() + 1))},
		},
		Transactions: tset,
	}
	for {
		block, found := tpt.miner.SolveBlock(block, target)
		if found {
			err = tpt.cs.AcceptBlock(block)
			if err != nil {
				tpt.t.Fatal(err)
			}
			break
		}
	}
	tpt.csUpdateWait()

	// Check that 'parent' and 'dependent' have been removed from the
	// transaction set, since conflict has made the confirmed set.
	if len(tpt.tpool.TransactionSet()) != 0 {
		tpt.t.Error("parent and dependent transaction are still in the pool after a conflict has been introduced, have", len(tset))
	}
}

// testDependentUpdates adds a parent transaction and a dependent transaction
// to the unconfirmed set. Then the parent transaction is added to the
// confirmed set but the dependent is not. A check is made to see that the
// dependent is still in the unconfirmed set.
func (tpt *tpoolTester) testDependentUpdates() {
	// Put two transactions, a parent and a dependent, into the transaction
	// pool. Then create a transaction that is in conflict with the parent.
	parent := tpt.emptyUnlockTransaction()
	dependent := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{
			types.SiacoinInput{
				ParentID: parent.SiacoinOutputID(0),
			},
		},
		MinerFees: []types.Currency{
			parent.SiacoinOutputs[0].Value,
		},
	}
	err := tpt.tpool.AcceptTransaction(parent)
	if err != nil {
		tpt.t.Fatal(err)
	}
	tpt.tpUpdateWait()
	err = tpt.tpool.AcceptTransaction(dependent)
	if err != nil {
		tpt.t.Fatal(err)
	}
	tpt.tpUpdateWait()

	// Mine a block to put the parent into the confirmed set.
	tset := tpt.tpool.TransactionSet()
	tset = tset[:len(tset)-1] // strip 'dependent'
	target := tpt.cs.CurrentTarget()
	block := types.Block{
		ParentID:  tpt.cs.CurrentBlock().ID(),
		Timestamp: types.Timestamp(time.Now().Unix()),
		MinerPayouts: []types.SiacoinOutput{
			types.SiacoinOutput{Value: types.CalculateCoinbase(tpt.cs.Height() + 1)},
		},
		Transactions: tset,
	}
	for {
		var found bool
		block, found = tpt.miner.SolveBlock(block, target)
		if found {
			err = tpt.cs.AcceptBlock(block)
			if err != nil {
				tpt.t.Fatal(err)
			}
			break
		}
	}
	tpt.csUpdateWait()

	// Check that 'parent' and 'dependent' have been removed from the
	// transaction set, since conflict has made the confirmed set.
	if len(tpt.tpool.TransactionSet()) != 1 {
		tpt.t.Error("dependent transaction does not remain unconfirmed after parent has been confirmed:", len(tset))
	}
}

// testRewinding adds transactions in a block, then removes the block and
// verifies that the transaction pool adds the block transactions.
func (tpt *tpoolTester) testRewinding() {
	// Put some transactions into the unconfirmed set.
	tpt.addSiacoinTransactionToPool()
	if len(tpt.tpool.TransactionSet()) == 0 {
		tpt.t.Fatal("transaction pool has no transactions")
	}

	// Prepare an empty block to cause a rewind (by forking).
	target := tpt.cs.CurrentTarget()
	forkStart := types.Block{
		ParentID:  tpt.cs.CurrentBlock().ID(),
		Timestamp: types.Timestamp(time.Now().Unix()),
		MinerPayouts: []types.SiacoinOutput{
			types.SiacoinOutput{Value: types.CalculateCoinbase(tpt.cs.Height() + 1)},
		},
	}
	for {
		var found bool
		forkStart, found = tpt.miner.SolveBlock(forkStart, target)
		if found {
			break
		}
	}

	// Mine a block with the transaction.
	for {
		_, found, err := tpt.miner.FindBlock()
		if err != nil {
			tpt.t.Fatal(err)
		}
		if found {
			break
		}
	}
	tpt.csUpdateWait()
	if len(tpt.tpool.TransactionSet()) != 0 {
		tpt.t.Fatal("tset should be empty after FindBlock()")
	}

	// Fork around the block with the transaction.
	err := tpt.cs.AcceptBlock(forkStart)
	if err != nil && err != modules.ErrNonExtendingBlock {
		tpt.t.Fatal(err)
	}
	target = tpt.cs.CurrentTarget()
	forkCommit := types.Block{
		ParentID:  forkStart.ID(),
		Timestamp: types.Timestamp(time.Now().Unix()),
		MinerPayouts: []types.SiacoinOutput{
			types.SiacoinOutput{Value: types.CalculateCoinbase(tpt.cs.Height() + 1)},
		},
	}
	for {
		var found bool
		forkCommit, found = tpt.miner.SolveBlock(forkCommit, target)
		if found {
			tpt.cs.AcceptBlock(forkCommit)
			break
		}
	}
	tpt.csUpdateWait()

	// Check that the transaction which was once confirmed but no longer is
	// confirmed is now unconfirmed.
	if len(tpt.tpool.TransactionSet()) == 0 {
		tpt.t.Error("tset should contain transactions that used to be confirmed but no longer are")
	}
}

// TestUpdateTransactionRemoval creates a tpoolTester and uses it to call
// tetsUpdateTransactionRemoval.
func TestUpdateTransactionRemoval(t *testing.T) {
	tpt := newTpoolTester("TestUpdateTransactionRemoval", t)
	tpt.testUpdateTransactionRemoval()
}

// TestDataTransactions creates a tpoolTester and uses it to call
// testDataTransactions.
func TestDataTransactions(t *testing.T) {
	tpt := newTpoolTester("TestDataTransactions", t)
	tpt.testDataTransactions()
}

// TestBlockConflicts creates a tpoolTester and uses it to call
// testBlockConflicts.
func TestBlockConflicts(t *testing.T) {
	tpt := newTpoolTester("TestBlockConflicts", t)
	tpt.testBlockConflicts()
}

// TestDependentUpdates creates a tpoolTester and uses it to call
// testDependentUpdates.
func TestDependentUpdates(t *testing.T) {
	tpt := newTpoolTester("TestDependentUpdates", t)
	tpt.testDependentUpdates()
}

// TestRewiding creates a tpoolTester and uses it to call testRewinding.
func TestRewinding(t *testing.T) {
	tpt := newTpoolTester("TestRewinding", t)
	tpt.testRewinding()
}
