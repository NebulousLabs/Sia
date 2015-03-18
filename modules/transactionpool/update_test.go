package transactionpool

import (
	"testing"
)

// testUpdateTransactionRemoval checks that when a transaction moves from the
// unconfirmed set into the confirmed set, the transaction gets correctly
// removed from the unconfirmed set.
func (tpt *tpoolTester) testUpdateTransactionRemoval() {
	// Create some unconfirmed transactions.
	tpt.addDependentSiacoinTransactionToPool()
	tset, err := tpt.tpool.TransactionSet()
	if err != nil {
		tpt.t.Error(err)
	}
	if len(tset) == 0 {
		tpt.t.Error("tset should have some transacitons")
	}

	// Mine a block to put the transactions into the confirmed set.
	for {
		_, found, err := tpt.miner.FindBlock()
		if err != nil {
			tpt.t.Fatal(err)
		}
		if found {
			break
		}
	}
	if err != nil {
		tpt.t.Error(err)
	}

	<-tpt.updateChan

	// Check that the transactions have been removed from the unconfirmed set.
	tset, err = tpt.tpool.TransactionSet()
	if err != nil {
		tpt.t.Error(err)
	}
	if len(tset) != 0 {
		tpt.t.Error("unconfirmed transaction set is not empty")
	}

	// Check that all of the internal maps are empty.
	id := tpt.tpool.mu.RLock()
	if len(tpt.tpool.transactions) != 0 {
		tpt.t.Error("a field wasn't properly emptied out")
	}
	if len(tpt.tpool.siacoinOutputs) != 0 {
		tpt.t.Error("a field wasn't properly emptied out")
	}
	if len(tpt.tpool.fileContracts) != 0 {
		tpt.t.Error("a field wasn't properly emptied out")
	}
	if len(tpt.tpool.siafundOutputs) != 0 {
		tpt.t.Error("a field wasn't properly emptied out")
	}
	if len(tpt.tpool.usedSiacoinOutputs) != 0 {
		tpt.t.Error("a field wasn't properly emptied out")
	}
	if len(tpt.tpool.newFileContracts) != 0 {
		tpt.t.Error("a field wasn't properly emptied out")
	}
	if len(tpt.tpool.newFileContracts) != 0 {
		tpt.t.Error("a field wasn't properly emptied out")
	}
	if len(tpt.tpool.fileContractTerminations) != 0 {
		tpt.t.Error("a field wasn't properly emptied out")
	}
	if len(tpt.tpool.storageProofs) != 0 {
		tpt.t.Error("a field wasn't properly emptied out")
	}
	if len(tpt.tpool.usedSiafundOutputs) != 0 {
		tpt.t.Error("a field wasn't properly emptied out")
	}
	tpt.tpool.mu.RUnlock(id)
}

/*
// testBlockConflicts adds a transaction and a dependent transaction to the
// transaction pool, and then adds a transaction to the blockchain that is in
// conflict with the first transaction. This should result in both the first
// transaction and the dependent transaction being removed from the transaction
// pool.
func (tpt *tpoolTester) testBlockConflicts() {
	// Prerequisite/TODO: transaction pool should be empty at this point.
	tset, err := tpt.TransactionSet()
	if err != nil {
		tpt.Error(err)
	}
	if len(tset) != 0 {
		tpt.Error("need tset length to be 0 for this test")
	}

	// Put two transactions, a parent and a dependent, into the transaction
	// pool. Then create a transaction that is in conflict with the parent.
	parentTxn, _ := tpt.addDependentSiacoinTransactionToPool()
	conflictTxn := parentTxn
	conflictTxn.MinerFees = append(conflictTxn.MinerFees, conflictTxn.SiacoinOutputs[0].Value)
	conflictTxn.SiacoinOutputs = nil

	// Mine a block with the conflict transaction and put it in the state.
	block := tpt.MineCurrentBlock([]consensus.Transaction{conflictTxn})
	err = tpt.AcceptBlock(block)
	if err != nil {
		tpt.Error(err)
	}

	// Update the transaction pool and check that both the parent and dependent
	// have been removed as a result of the conflict making it into the
	// blockchain.
	tset, err = tpt.TransactionSet()
	if err != nil {
		tpt.Error(err)
	}
	if len(tset) != 0 {
		tpt.Error("conflict transactions not all cleared from transaction pool")
	}
}

// testDependentUpdates adds a transaction and a dependent transaction to the
// transaction pool, and then adds the first transaction to the blockchain. The
// dependent transaction should be the only transaction in the transaction pool
// after that point.
func (tpt *tpoolTester) testDependentUpdates() {
	// Prerequisite/TODO: transaction pool should be empty at this point.
	tset, err := tpt.TransactionSet()
	if err != nil {
		tpt.Error(err)
	}
	if len(tset) != 0 {
		tpt.Error("need tset length to be 0 for this test")
	}

	parentTxn, dependentTxn := tpt.addDependentSiacoinTransactionToPool()

	// Mine a block with the parent transaction but not the dependent.
	block := tpt.MineCurrentBlock([]consensus.Transaction{parentTxn})
	err = tpt.AcceptBlock(block)
	if err != nil {
		tpt.Error(err)
	}

	// Update the transaction pool and check that only the dependent
	// transaction remains.
	tset, err = tpt.TransactionSet()
	if err != nil {
		tpt.Error(err)
	}
	if len(tset) != 1 {
		tpt.Error("conflict transactions not all cleared from transaction pool")
	}
	if crypto.HashObject(tset[0]) != crypto.HashObject(dependentTxn) {
		tpt.Error("dependent transaction is not the transaction that remains")
	}
}

// testRewinding adds transactions in a block, then removes the block and
// verifies that the transaction pool adds the block transactions.
func (tpt *tpoolTester) testRewinding() {
	// Prerequisite/TODO: transaction pool should be empty at this point.
	tset, err := tpt.TransactionSet()
	if err != nil {
		tpt.Error(err)
	}
	if len(tset) != 0 {
		tpt.Error("need tset length to be 0 for this test")
	}

	// Mine a block with a transaction.
	sci, value := tpt.FindSpendableSiacoinInput()
	txn := tpt.AddSiacoinInputToTransaction(consensus.Transaction{}, sci)
	txn.MinerFees = append(txn.MinerFees, value)
	block := tpt.MineCurrentBlock([]consensus.Transaction{txn})
	err = tpt.AcceptBlock(block)
	if err != nil {
		tpt.Error(err)
	}

	// Rewind the block, update the transaction pool, and check that the
	// transaction was added to the transaction pool.
	tpt.RewindABlock()
	tset, err = tpt.TransactionSet()
	if err != nil {
		tpt.Error(err)
	}
	if len(tset) != 1 {
		tpt.Fatal("expecting new transaction after rewind")
	}
	if crypto.HashObject(tset[0]) != crypto.HashObject(txn) {
		tpt.Error("dependent transaction is not the transaction that remains")
	}
}
*/

// TestUpdateTransactionRemoval creates a tpoolTester and uses it to call
// tetsUpdateTransactionRemoval.
func TestUpdateTransactionRemoval(t *testing.T) {
	tpt := newTpoolTester("Transaction Pool - TestUpdateTransactionRemoval", t)
	tpt.testUpdateTransactionRemoval()
}

/*
// TestBlockConflicts creates a tpoolTester and uses it to call
// testBlockConflicts.
func TestBlockConflicts(t *testing.T) {
	tpt := newTpoolTester("Transaction Pool - TestBlockConflicts", t)
	tpt.testBlockConflicts()
}

// TestDependentUpdates creates a tpoolTester and uses it to call
// testDependentUpdates.
func TestDependentUpdates(t *testing.T) {
	tpt := newTpoolTester("Transaction Pool - TestDependentUpdates", t)
	tpt.testDependentUpdates()
}

// TestRewinding creates a tpoolTester and uses it to call testRewinding.
func TestRewinding(t *testing.T) {
	tpt := newTpoolTester("Transaction Pool - TestRewinding", t)
	tpt.testRewinding()
}
*/
