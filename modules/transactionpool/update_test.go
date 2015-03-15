package transactionpool

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
)

// testUpdateTransactionRemoval puts several transactions into the transaction
// pool, and then into a block, then puts the block into the state. After the
// transaction pool updates, the transactions should have been removed from the
// transaction pool.
func (tpt *TpoolTester) testUpdateTransactionRemoval() {
	// Add some transactions to the pool and then get the transaction set.
	tpt.addDependentSiacoinTransactionToPool()
	tset, err := tpt.TransactionSet()
	id := tpt.mu.RLock()
	if err != nil {
		tpt.Error(err)
	}
	if len(tset) == 0 {
		tpt.Error("tset should have some transacitons")
	}
	tpt.mu.RUnlock(id)

	// TODO: Add all other types of transactions.

	// Mine a block that has the transactions.
	b := tpt.MineCurrentBlock(tset)
	err = tpt.AcceptBlock(b)
	if err != nil {
		tpt.Error(err)
	}

	// Call update and verify that the new transaction set is empty.
	tset, err = tpt.TransactionSet()
	if err != nil {
		tpt.Error(err)
	}
	id = tpt.mu.RLock()
	if len(tset) != 0 {
		tpt.Error("tset should not have any transactions")
	}
	tpt.mu.RUnlock(id)

	// Check that all of the internal maps are also empty.
	id = tpt.mu.RLock()
	if len(tpt.transactions) != 0 {
		tpt.Error("a field wasn't properly emptied out")
	}
	if len(tpt.siacoinOutputs) != 0 {
		tpt.Error("a field wasn't properly emptied out")
	}
	if len(tpt.fileContracts) != 0 {
		tpt.Error("a field wasn't properly emptied out")
	}
	if len(tpt.siafundOutputs) != 0 {
		tpt.Error("a field wasn't properly emptied out")
	}
	if len(tpt.usedSiacoinOutputs) != 0 {
		tpt.Error("a field wasn't properly emptied out")
	}
	if len(tpt.newFileContracts) != 0 {
		tpt.Error("a field wasn't properly emptied out")
	}
	if len(tpt.newFileContracts) != 0 {
		tpt.Error("a field wasn't properly emptied out")
	}
	if len(tpt.fileContractTerminations) != 0 {
		tpt.Error("a field wasn't properly emptied out")
	}
	if len(tpt.storageProofs) != 0 {
		tpt.Error("a field wasn't properly emptied out")
	}
	if len(tpt.usedSiafundOutputs) != 0 {
		tpt.Error("a field wasn't properly emptied out")
	}
	tpt.mu.RUnlock(id)
}

// testBlockConflicts adds a transaction and a dependent transaction to the
// transaction pool, and then adds a transaction to the blockchain that is in
// conflict with the first transaction. This should result in both the first
// transaction and the dependent transaction being removed from the transaction
// pool.
func (tpt *TpoolTester) testBlockConflicts() {
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
func (tpt *TpoolTester) testDependentUpdates() {
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
func (tpt *TpoolTester) testRewinding() {
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

// TestUpdateTransactionRemoval creates a TpoolTester and uses it to call
// tetsUpdateTransactionRemoval.
func TestUpdateTransactionRemoval(t *testing.T) {
	tpt := CreateTpoolTester(t)
	tpt.testUpdateTransactionRemoval()
}

// TestBlockConflicts creates a TpoolTester and uses it to call
// testBlockConflicts.
func TestBlockConflicts(t *testing.T) {
	tpt := CreateTpoolTester(t)
	tpt.testBlockConflicts()
}

// TestDependentUpdates creates a TpoolTester and uses it to call
// testDependentUpdates.
func TestDependentUpdates(t *testing.T) {
	tpt := CreateTpoolTester(t)
	tpt.testDependentUpdates()
}

// TestRewinding creates a TpoolTester and uses it to call testRewinding.
func TestRewinding(t *testing.T) {
	tpt := CreateTpoolTester(t)
	tpt.testRewinding()
}
