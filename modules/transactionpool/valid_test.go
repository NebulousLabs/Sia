package transactionpool

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
)

// addConflictingSiacoinTransactionToPool creates a valid transaction, adds it
// to the pool, and then tries to submit a transaction that uses the same
// outputs and checks that the double-spend attempt is caught by the pool.
func (tpt *tpoolTester) addConflictingSiacoinTransaction() {
	// Send money to an anyone-can-spend address.
	anySpend := consensus.UnlockConditions{}.UnlockHash()
	txn, err := tpt.wallet.SpendCoins(consensus.NewCurrency64(1), anySpend)
	if err != nil {
		tpt.t.Fatal(err)
	}
	outputID := txn.SiacoinOutputID(0)

	// Create a transaction spending the coins.
	txn = consensus.Transaction{
		SiacoinInputs: []consensus.SiacoinInput{
			consensus.SiacoinInput{
				ParentID: outputID,
			},
		},
		SiacoinOutputs: []consensus.SiacoinOutput{
			consensus.SiacoinOutput{
				Value: consensus.NewCurrency64(1),
			},
		},
	}

	// Try to add a double spend transaction to the pool.
	err = tpt.tpool.AcceptTransaction(txn)
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
