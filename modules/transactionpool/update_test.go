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
func (tpt *tpoolTester) testUpdateTransactionRemoval() {
	// Add some transactions to the pool and then get the transaction set.
	tpt.addDependentSiacoinTransactionToPool()
	tset, err := tpt.transactionPool.TransactionSet()
	if err != nil {
		tpt.assistant.Error(err)
	}
	if len(tset) == 0 {
		tpt.assistant.Error("tset should have some transacitons")
	}

	// TODO: Add all other types of transactions.

	// Mine a block that has the transactions.
	b, err := tpt.assistant.MineCurrentBlock(tset)
	if err != nil {
		tpt.assistant.Error(err)
	}
	err = tpt.assistant.AcceptBlock(b)
	if err != nil {
		tpt.assistant.Error(err)
	}

	// Call update and verify that the new transaction set is empty.
	tpt.transactionPool.update()
	tset, err = tpt.transactionPool.TransactionSet()
	if err != nil {
		tpt.assistant.Error(err)
	}
	if len(tset) != 0 {
		tpt.assistant.Error("tset should not have any transactions")
	}

	// Check that all of the internal maps are also empty.
	if len(tpt.transactionPool.transactions) != 0 {
		tpt.assistant.Error("a field wasn't properly emptied out")
	}
	if len(tpt.transactionPool.siacoinOutputs) != 0 {
		tpt.assistant.Error("a field wasn't properly emptied out")
	}
	if len(tpt.transactionPool.fileContracts) != 0 {
		tpt.assistant.Error("a field wasn't properly emptied out")
	}
	if len(tpt.transactionPool.siafundOutputs) != 0 {
		tpt.assistant.Error("a field wasn't properly emptied out")
	}
	if len(tpt.transactionPool.usedSiacoinOutputs) != 0 {
		tpt.assistant.Error("a field wasn't properly emptied out")
	}
	if len(tpt.transactionPool.newFileContracts) != 0 {
		tpt.assistant.Error("a field wasn't properly emptied out")
	}
	if len(tpt.transactionPool.newFileContracts) != 0 {
		tpt.assistant.Error("a field wasn't properly emptied out")
	}
	if len(tpt.transactionPool.fileContractTerminations) != 0 {
		tpt.assistant.Error("a field wasn't properly emptied out")
	}
	if len(tpt.transactionPool.storageProofs) != 0 {
		tpt.assistant.Error("a field wasn't properly emptied out")
	}
	if len(tpt.transactionPool.usedSiafundOutputs) != 0 {
		tpt.assistant.Error("a field wasn't properly emptied out")
	}
}

// testBlockConflicts adds a transaction and a dependent transaction to the
// transaction pool, and then adds a transaction to the blockchain that is in
// conflict with the first transaction. This should result in both the first
// transaction and the dependent transaction being removed from the transaction
// pool.
func (tpt *tpoolTester) testBlockConflicts() {
	// Prerequisite/TODO: transaction pool should be empty at this point.
	tset, err := tpt.transactionPool.TransactionSet()
	if err != nil {
		tpt.assistant.Error(err)
	}
	if len(tset) != 0 {
		tpt.assistant.Error("need tset length to be 0 for this test")
	}

	// Put two transactions, a parent and a dependent, into the transaction
	// pool. Then create a transaction that is in conflict with the parent.
	parentTxn, _ := tpt.addDependentSiacoinTransactionToPool()
	conflictTxn := parentTxn
	conflictTxn.MinerFees = append(conflictTxn.MinerFees, conflictTxn.SiacoinOutputs[0].Value)
	conflictTxn.SiacoinOutputs = nil

	// Mine a block with the conflict transaction and put it in the state.
	block, err := tpt.assistant.MineCurrentBlock([]consensus.Transaction{conflictTxn})
	if err != nil {
		tpt.assistant.Error(err)
	}
	err = tpt.assistant.AcceptBlock(block)
	if err != nil {
		tpt.assistant.Error(err)
	}

	// Update the transaction pool and check that both the parent and dependent
	// have been removed as a result of the conflict making it into the
	// blockchain.
	tpt.transactionPool.update()
	tset, err = tpt.transactionPool.TransactionSet()
	if err != nil {
		tpt.assistant.Error(err)
	}
	if len(tset) != 0 {
		tpt.assistant.Error("conflict transactions not all cleared from transaction pool")
	}
}

// testDependentUpdates adds a transaction and a dependent transaction to the
// transaction pool, and then adds the first transaction to the blockchain. The
// dependent transaction should be the only transaction in the transaction pool
// after that point.
func (tpt *tpoolTester) testDependentUpdates() {
	// Prerequisite/TODO: transaction pool should be empty at this point.
	tset, err := tpt.transactionPool.TransactionSet()
	if err != nil {
		tpt.assistant.Error(err)
	}
	if len(tset) != 0 {
		tpt.assistant.Error("need tset length to be 0 for this test")
	}

	parentTxn, dependentTxn := tpt.addDependentSiacoinTransactionToPool()

	// Mine a block with the parent transaction but not the dependent.
	block, err := tpt.assistant.MineCurrentBlock([]consensus.Transaction{parentTxn})
	if err != nil {
		tpt.assistant.Error(err)
	}
	err = tpt.assistant.AcceptBlock(block)
	if err != nil {
		tpt.assistant.Error(err)
	}

	// Update the transaction pool and check that only the dependent
	// transaction remains.
	tpt.transactionPool.update()
	tset, err = tpt.transactionPool.TransactionSet()
	if err != nil {
		tpt.assistant.Error(err)
	}
	if len(tset) != 1 {
		tpt.assistant.Error("conflict transactions not all cleared from transaction pool")
	}
	if crypto.HashObject(tset[0]) != crypto.HashObject(dependentTxn) {
		tpt.assistant.Error("dependent transaction is not the transaction that remains")
	}
}

// testRewinding adds transactions in a block, then removes the block and
// verifies that the transaction pool adds the block transactions.
func (tpt *tpoolTester) testRewinding() {
	// Prerequisite/TODO: transaction pool should be empty at this point.
	tset, err := tpt.transactionPool.TransactionSet()
	if err != nil {
		tpt.assistant.Error(err)
	}
	if len(tset) != 0 {
		tpt.assistant.Error("need tset length to be 0 for this test")
	}

	// Mine a block with a transaction.
	sci, value := tpt.assistant.FindSpendableSiacoinInput()
	txn := tpt.assistant.AddSiacoinInputToTransaction(consensus.Transaction{}, sci)
	txn.MinerFees = append(txn.MinerFees, value)
	block, err := tpt.assistant.MineCurrentBlock([]consensus.Transaction{txn})
	if err != nil {
		tpt.assistant.Error(err)
	}
	err = tpt.assistant.AcceptBlock(block)
	if err != nil {
		tpt.assistant.Error(err)
	}

	// Rewind the block, update the transaction pool, and check that the
	// transaction was added to the transaction pool.
	tpt.transactionPool.update()
	tpt.assistant.RewindABlock()
	tpt.transactionPool.update()
	tset, err = tpt.transactionPool.TransactionSet()
	if err != nil {
		tpt.assistant.Error(err)
	}
	if len(tset) != 1 {
		tpt.assistant.Fatal("expecting new transaction after rewind")
	}
	if crypto.HashObject(tset[0]) != crypto.HashObject(txn) {
		tpt.assistant.Error("dependent transaction is not the transaction that remains")
	}
}

// TestUpdateTransactionRemoval creates a tpoolTester and uses it to call
// tetsUpdateTransactionRemoval.
func TestUpdateTransactionRemoval(t *testing.T) {
	tpt := CreateTpoolTester(t)
	tpt.testUpdateTransactionRemoval()
}

// TestBlockConflicts creates a tpoolTester and uses it to call
// testBlockConflicts.
func TestBlockConflicts(t *testing.T) {
	tpt := CreateTpoolTester(t)
	tpt.testBlockConflicts()
}

// TestDependentUpdates creates a tpoolTester and uses it to call
// testDependentUpdates.
func TestDependentUpdates(t *testing.T) {
	tpt := CreateTpoolTester(t)
	tpt.testDependentUpdates()
}

// TestRewinding creates a tpoolTester and uses it to call testRewinding.
func TestRewinding(t *testing.T) {
	tpt := CreateTpoolTester(t)
	tpt.testRewinding()
}
