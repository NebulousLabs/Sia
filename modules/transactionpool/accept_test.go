package transactionpool

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
)

// addSiacoinTransactionToPool creates a transaction with siacoin outputs and
// adds them to the pool, returning the transaction.
func (tpt *tpoolTester) addSiacoinTransactionToPool() (txn consensus.Transaction) {
	// SpendCoins will automatically add transaction(s) to the transaction pool.
	// They will contain siacoin output(s).
	txn, err := tpt.wallet.SpendCoins(consensus.NewCurrency64(1), consensus.ZeroUnlockHash)
	if err != nil {
		tpt.t.Fatal(err)
	}
	<-tpt.updateChan

	return
}

// addDependentSiacoinTransactionToPool adds a transaction to the pool with a
// siacoin output, and then adds a second transaction to the pool that requires
// the unconfirmed siacoin output.
func (tpt *tpoolTester) addDependentSiacoinTransactionToPool() (firstTxn, dependentTxn consensus.Transaction) {
	// Get an address to receive coins.
	addr, _, err := tpt.wallet.CoinAddress()
	if err != nil {
		tpt.t.Fatal(err)
	}

	// SpendCoins will automatically add transaction(s) to the transaction
	// pool. They will contain siacoin output(s). We send all of our coins to
	// ourself to guarantee that the next transaction will depend on an
	// existing unconfirmed transaction.
	balance := tpt.wallet.Balance(false)
	firstTxn, err = tpt.wallet.SpendCoins(balance, addr)
	if err != nil {
		tpt.t.Fatal(err)
	}
	<-tpt.updateChan

	// Send the full balance to ourselves again. The second transaction will
	// necesarily require the first transaction as a dependency, since we're
	// sending all of the coins again.
	dependentTxn, err = tpt.wallet.SpendCoins(balance, addr)
	if err != nil {
		tpt.t.Fatal(err)
	}
	<-tpt.updateChan

	return
}

// TestAddSiacoinTransactionToPool creates a tpoolTester and uses it to call
// addSiacoinTransactionToPool.
func TestAddSiacoinTransactionToPool(t *testing.T) {
	tpt := newTpoolTester("TransactionPool - TestAddSiacoinTransactionToPool", t)
	tpt.addSiacoinTransactionToPool()
}

// TestAddDependentSiacoinTransactionToPool creates a tpoolTester and uses it
// to cal addDependentSiacoinTransactionToPool.
func TestAddDependentSiacoinTransactionToPool(t *testing.T) {
	tpt := newTpoolTester("TransactionPool - TestAddDependentSiacoinTransactionToPool", t)
	tpt.addDependentSiacoinTransactionToPool()
}

// TestDuplicateTransaction checks that a duplicate transaction error is
// triggered when duplicate transactions are added to the transaction pool.
// This test won't be needed after the duplication prevention mechanism is
// removed, and that will be removed after fees are required in all
// transactions submitted to the pool.
func TestDuplicateTransaction(t *testing.T) {
	tpt := newTpoolTester("TransactionPool - TestDuplicateTransaction", t)
	txn := tpt.addSiacoinTransactionToPool()
	err := tpt.tpool.AcceptTransaction(txn)
	if err != ErrDuplicate {
		t.Fatal("expecting ErrDuplicate got:", err)
	}
}
