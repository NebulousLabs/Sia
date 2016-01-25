package transactionpool

import (
	"crypto/rand"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestIntegrationAcceptTransactionSet probes the AcceptTransactionSet method
// of the transaction pool.
func TestIntegrationAcceptTransactionSet(t *testing.T) {
	// Create a transaction pool tester.
	tpt, err := createTpoolTester("TestIntegrationAcceptTransactionSet")
	if err != nil {
		t.Fatal(err)
	}

	// Check that the transaction pool is empty.
	if len(tpt.tpool.transactionSets) != 0 {
		t.Error("transaction pool is not empty")
	}

	// Create a valid transaction set using the wallet.
	txns, err := tpt.wallet.SendSiacoins(types.NewCurrency64(100), types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tpt.tpool.transactionSets) != 1 {
		t.Error("sending coins did not increase the transaction sets by 1")
	}

	// Submit the transaction set again to trigger a duplication error.
	err = tpt.tpool.AcceptTransactionSet(txns)
	if err != modules.ErrDuplicateTransactionSet {
		t.Error(err)
	}

	// Mine a block and check that the transaction pool gets emptied.
	block, _ := tpt.miner.FindBlock()
	err = tpt.cs.AcceptBlock(block)
	if err != nil {
		t.Fatal(err)
	}
	if len(tpt.tpool.TransactionList()) != 0 {
		t.Error("transaction pool was not emptied after mining a block")
	}

	// Try to resubmit the transaction set to verify
	err = tpt.tpool.AcceptTransactionSet(txns)
	if err == nil {
		t.Error("transaction set was supposed to be rejected")
	}
}

// TestIntegrationConflictingTransactionSets tries to add two transaction sets
// to the transaction pool that are each legal individually, but double spend
// an output.
func TestIntegrationConflictingTransactionSets(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Create a transaction pool tester.
	tpt, err := createTpoolTester("TestIntegrationConflictingTransactionSets")
	if err != nil {
		t.Fatal(err)
	}

	// Fund a partial transaction.
	fund := types.NewCurrency64(30e6)
	txnBuilder := tpt.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(fund)
	if err != nil {
		t.Fatal(err)
	}
	// wholeTransaction is set to false so that we can use the same signature
	// to create a double spend.
	txnSet, err := txnBuilder.Sign(false)
	if err != nil {
		t.Fatal(err)
	}
	txnSetDoubleSpend := make([]types.Transaction, len(txnSet))
	copy(txnSetDoubleSpend, txnSet)

	// There are now two sets of transactions that are signed and ready to
	// spend the same output. Have one spend the money in a miner fee, and the
	// other create a siacoin output.
	txnIndex := len(txnSet) - 1
	txnSet[txnIndex].MinerFees = append(txnSet[txnIndex].MinerFees, fund)
	txnSetDoubleSpend[txnIndex].SiacoinOutputs = append(txnSetDoubleSpend[txnIndex].SiacoinOutputs, types.SiacoinOutput{Value: fund})

	// Add the first and then the second txn set.
	err = tpt.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		t.Error(err)
	}
	err = tpt.tpool.AcceptTransactionSet(txnSetDoubleSpend)
	if err == nil {
		t.Error("transaction should not have passed inspection")
	}

	// Purge and try the sets in the reverse order.
	tpt.tpool.PurgeTransactionPool()
	err = tpt.tpool.AcceptTransactionSet(txnSetDoubleSpend)
	if err != nil {
		t.Error(err)
	}
	err = tpt.tpool.AcceptTransactionSet(txnSet)
	if err == nil {
		t.Error("transaction should not have passed inspection")
	}
}

// TestIntegrationCheckMinerFees probes the checkMinerFees method of the
// transaction pool.
func TestIntegrationCheckMinerFees(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Create a transaction pool tester.
	tpt, err := createTpoolTester("TestIntegrationCheckMinerFees")
	if err != nil {
		t.Fatal(err)
	}

	// Fill the transaction pool to the fee limit.
	for i := 0; i < TransactionPoolSizeForFee/10e3; i++ {
		arbData := make([]byte, 10e3)
		copy(arbData, modules.PrefixNonSia[:])
		_, err = rand.Read(arbData[100:116]) // prevents collisions with other transacitons in the loop.
		if err != nil {
			t.Fatal(err)
		}
		txn := types.Transaction{ArbitraryData: [][]byte{arbData}}
		err := tpt.tpool.AcceptTransactionSet([]types.Transaction{txn})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Add another transaction, this one should fail for having too few fees.
	err = tpt.tpool.AcceptTransactionSet([]types.Transaction{{}})
	if err != errLowMinerFees {
		t.Error(err)
	}

	// Add a transaction that has sufficient fees.
	_, err = tpt.wallet.SendSiacoins(types.NewCurrency64(100), types.UnlockHash{})
	if err != nil {
		t.Error(err)
	}

	// TODO: fill the pool up all the way and try again.
}

// TestTransactionSuperset submits a single transaction to the network,
// followed by a transaction set containing that single transaction.
func TestIntegrationTransactionSuperset(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Create a transaction pool tester.
	tpt, err := createTpoolTester("TestTransactionSuperset")
	if err != nil {
		t.Fatal(err)
	}

	// Fund a partial transaction.
	fund := types.NewCurrency64(30e6)
	txnBuilder := tpt.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(fund)
	if err != nil {
		t.Fatal(err)
	}
	txnBuilder.AddMinerFee(fund)
	// wholeTransaction is set to false so that we can use the same signature
	// to create a double spend.
	txnSet, err := txnBuilder.Sign(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(txnSet) <= 1 {
		t.Fatal("test is invalid unless the transaction set has two or more transactions")
	}
	// Check that the second transaction is dependent on the first.
	err = tpt.tpool.AcceptTransactionSet(txnSet[1:])
	if err == nil {
		t.Fatal("transaction set must have dependent transactions")
	}

	// Submit the first transaction in the set to the transaction pool.
	err = tpt.tpool.AcceptTransactionSet(txnSet[:1])
	if err != nil {
		t.Fatal("first transaction in the transaction set was not valid?")
	}
	err = tpt.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		t.Fatal("super setting is not working:", err)
	}

	// Try resubmitting the individual transaction and the superset, a
	// duplication error should be returned for each case.
	err = tpt.tpool.AcceptTransactionSet(txnSet[:1])
	if err != modules.ErrDuplicateTransactionSet {
		t.Fatal(err)
	}
	err = tpt.tpool.AcceptTransactionSet(txnSet)
	if err != modules.ErrDuplicateTransactionSet {
		t.Fatal("super setting is not working:", err)
	}
}

// TestIntegrationTransactionChild submits a single transaction to the network,
// followed by a child transaction.
func TestIntegrationTransactionChild(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Create a transaction pool tester.
	tpt, err := createTpoolTester("TestTransactionChild")
	if err != nil {
		t.Fatal(err)
	}

	// Fund a partial transaction.
	fund := types.NewCurrency64(30e6)
	txnBuilder := tpt.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(fund)
	if err != nil {
		t.Fatal(err)
	}
	txnBuilder.AddMinerFee(fund)
	// wholeTransaction is set to false so that we can use the same signature
	// to create a double spend.
	txnSet, err := txnBuilder.Sign(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(txnSet) <= 1 {
		t.Fatal("test is invalid unless the transaction set has two or more transactions")
	}
	// Check that the second transaction is dependent on the first.
	err = tpt.tpool.AcceptTransactionSet([]types.Transaction{txnSet[1]})
	if err == nil {
		t.Fatal("transaction set must have dependent transactions")
	}

	// Submit the first transaction in the set to the transaction pool.
	err = tpt.tpool.AcceptTransactionSet(txnSet[:1])
	if err != nil {
		t.Fatal("first transaction in the transaction set was not valid?")
	}
	err = tpt.tpool.AcceptTransactionSet(txnSet[1:])
	if err != nil {
		t.Fatal("child transaction not seen as valid")
	}
}

// TestIntegrationNilAccept tries submitting a nil transaction set and a 0-len
// transaction set to the transaction pool.
func TestIntegrationNilAccept(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Create a transaction pool tester.
	tpt, err := createTpoolTester("TestTransactionChild")
	if err != nil {
		t.Fatal(err)
	}
	err = tpt.tpool.AcceptTransactionSet(nil)
	if err == nil {
		t.Error("no error returned when submitting nothing to the transaction pool")
	}
	err = tpt.tpool.AcceptTransactionSet([]types.Transaction{})
	if err == nil {
		t.Error("no error returned when submitting nothing to the transaction pool")
	}
}
