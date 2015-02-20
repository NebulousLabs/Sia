package transactionpool

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
)

// addSiacoinTransactionToPool creates a transaction with a single siacoin
// input and output and adds it to the transcation pool, returning the
// transaction that was created and added.
func (tpt *TpoolTester) addSiacoinTransactionToPool() (txn consensus.Transaction) {
	// Add a siacoin input to the transaction.
	siacoinInput, value := tpt.FindSpendableSiacoinInput()
	txn = tpt.AddSiacoinInputToTransaction(consensus.Transaction{}, siacoinInput)

	// Add a siacoin output to the transaction.
	sco := consensus.SiacoinOutput{
		Value:      value,
		UnlockHash: tpt.UnlockHash,
	}
	txn.SiacoinOutputs = append(txn.SiacoinOutputs, sco)

	// Put the transaction into the transaction pool.
	err := tpt.AcceptTransaction(txn)
	if err != nil {
		tpt.Error(err)
	}

	return
}

// addDependentSiacoinTransactionToPool adds a transaction to the pool with a
// siacoin output, and then adds a second transaction to the pool that requires
// the unconfirmed siacoin output.
func (tpt *TpoolTester) addDependentSiacoinTransactionToPool() (firstTxn, dependentTxn consensus.Transaction) {
	// Grab the first transaction and then create a second transaction.
	firstTxn = tpt.addSiacoinTransactionToPool()
	dependentTxn = consensus.Transaction{}
	sci := consensus.SiacoinInput{
		ParentID:         firstTxn.SiacoinOutputID(0),
		UnlockConditions: tpt.UnlockConditions,
	}
	dependentTxn = tpt.AddSiacoinInputToTransaction(dependentTxn, sci)
	dependentTxn.MinerFees = append(dependentTxn.MinerFees, firstTxn.SiacoinOutputs[0].Value)

	err := tpt.AcceptTransaction(dependentTxn)
	if err != nil {
		tpt.Error(err)
	}

	return
}

// TestAddSiacoinTransactionToPool creates a TpoolTester and uses it to call
// addSiacoinTransactionToPool.
func TestAddSiacoinTransactionToPool(t *testing.T) {
	tpt := CreateTpoolTester(t)
	tpt.addSiacoinTransactionToPool()
}

// TestAddDependentSiacoinTransactionToPool creates a TpoolTester and uses it
// to cal addDependentSiacoinTransactionToPool.
func TestAddDependentSiacoinTransactionToPool(t *testing.T) {
	tpt := CreateTpoolTester(t)
	tpt.addDependentSiacoinTransactionToPool()
}
