package transactionpool

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestArbDataOnly tries submitting a transaction with only arbitrary data to
// the transaction pool. Then a block is mined, putting the transaction on the
// blockchain. The arb data transaction should no longer be in the transaction
// pool.
func TestArbDataOnly(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	tpt, err := createTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()
	txn := types.Transaction{
		ArbitraryData: [][]byte{
			append(modules.PrefixNonSia[:], []byte("arb-data")...),
		},
	}
	err = tpt.tpool.AcceptTransactionSet([]types.Transaction{txn})
	if err != nil {
		t.Fatal(err)
	}
	if len(tpt.tpool.TransactionList()) != 1 {
		t.Error("expecting to see a transaction in the transaction pool")
	}
	_, err = tpt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(tpt.tpool.TransactionList()) != 0 {
		t.Error("transaction was not cleared from the transaction pool")
	}
}

// TestValidRevertedTransaction verifies that if a transaction appears in a
// block's reverted transactions, it is added correctly to the pool.
func TestValidRevertedTransaction(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	tpt, err := createTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()

	tpt2, err := blankTpoolTester(t.Name() + "-tpt2")
	if err != nil {
		t.Fatal(err)
	}
	defer tpt2.Close()

	// connect the testers and wait for them to have the same current block
	err = tpt2.gateway.Connect(tpt.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	success := false
	for start := time.Now(); time.Since(start) < time.Minute; time.Sleep(time.Millisecond * 100) {
		if tpt.cs.CurrentBlock().ID() == tpt2.cs.CurrentBlock().ID() {
			success = true
			break
		}
	}
	if !success {
		t.Fatal("testers did not have the same block height after one minute")
	}

	// disconnect the testers
	err = tpt2.gateway.Disconnect(tpt.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}

	// make some transactions on tpt
	var txnSets [][]types.Transaction
	for i := 0; i < 5; i++ {
		txns, err := tpt.wallet.SendSiacoins(types.SiacoinPrecision.Mul64(1000), types.UnlockHash{})
		if err != nil {
			t.Fatal(err)
		}
		txnSets = append(txnSets, txns)
	}
	// mine some blocks to cause a re-org
	for i := 0; i < 3; i++ {
		_, err = tpt.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	// put tpt2 at a higher height
	for i := 0; i < 10; i++ {
		_, err = tpt2.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// connect the testers and wait for them to have the same current block
	err = tpt.gateway.Connect(tpt2.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	success = false
	for start := time.Now(); time.Since(start) < time.Minute; time.Sleep(time.Millisecond * 100) {
		if tpt.cs.CurrentBlock().ID() == tpt2.cs.CurrentBlock().ID() {
			success = true
			break
		}
	}
	if !success {
		t.Fatal("testers did not have the same block height after one minute")
	}

	// verify the transaction pool still has the reorged txns
	for _, txnSet := range txnSets {
		for _, txn := range txnSet {
			_, _, exists := tpt.tpool.Transaction(txn.ID())
			if !exists {
				t.Error("Transaction was not re-added to the transaction pool after being re-orged out of the blockchain:", txn.ID())
			}
		}
	}

	// Try to get the transactoins into a block.
	_, err = tpt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(tpt.tpool.TransactionList()) != 0 {
		t.Error("Does not seem that the transactions were added to the transaction pool.")
	}
}

// TestTransactionPoolPruning verifies that the transaction pool correctly
// prunes transactions older than maxTxnAge.
func TestTransactionPoolPruning(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	tpt, err := createTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()
	tpt2, err := blankTpoolTester(t.Name() + "-tpt2")
	if err != nil {
		t.Fatal(err)
	}
	defer tpt2.Close()

	// connect the testers and wait for them to have the same current block
	err = tpt2.gateway.Connect(tpt.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	success := false
	for start := time.Now(); time.Since(start) < time.Minute; time.Sleep(time.Millisecond * 100) {
		if tpt.cs.CurrentBlock().ID() == tpt2.cs.CurrentBlock().ID() {
			success = true
			break
		}
	}
	if !success {
		t.Fatal("testers did not have the same block height after one minute")
	}

	// disconnect tpt, create an unconfirmed transaction on tpt, mine maxTxnAge
	// blocks on tpt2 and reconnect. The unconfirmed transactions should be
	// removed from tpt's pool.
	err = tpt2.gateway.Disconnect(tpt.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	txns, err := tpt.wallet.SendSiacoins(types.SiacoinPrecision.Mul64(1000), types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	for i := types.BlockHeight(0); i < maxTxnAge+1; i++ {
		_, err = tpt2.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// reconnect the testers
	err = tpt.gateway.Connect(tpt2.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	success = false
	for start := time.Now(); time.Since(start) < time.Minute; time.Sleep(time.Millisecond * 100) {
		if tpt.cs.CurrentBlock().ID() == tpt2.cs.CurrentBlock().ID() {
			success = true
			break
		}
	}
	if !success {
		t.Fatal("testers did not have the same block height after one minute")
	}

	for _, txn := range txns {
		_, _, exists := tpt.tpool.Transaction(txn.ID())
		if exists {
			t.Fatal("transaction pool had a transaction that should have been pruned")
		}
	}
	if len(tpt.tpool.TransactionList()) != 0 {
		t.Fatal("should have no unconfirmed transactions")
	}
}
