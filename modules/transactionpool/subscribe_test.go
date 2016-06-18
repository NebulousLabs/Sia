package transactionpool

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"testing"
)

// mockSubscriber receives transactions from the transaction pool it is
// subscribed to, retaining them in the order they were received.
type mockSubscriber struct {
	txns []types.Transaction
}

// ReceiveUpdatedUnconfirmedTransactions receives transactinos from the
// transaction pool and stores them in the order they were received. 
// This method allows *mockSubscriber to satisfy the
// modules.TransactionPoolSubscriber interface.
func (ms *mockSubscriber) ReceiveUpdatedUnconfirmedTransactions(txns []types.Transaction, _ modules.ConsensusChange) {
	ms.txns = append(ms.txns, txns...)
}

// TestSubscription checks that calling Unsubscribe on a mockSubscriber
// shortens the list of subscribers to the transaction pool by 1 (doesn't
// actually check that the mockSubscriber was the one unsubscribed).
func TestSubscription(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	tpt, err  := createTpoolTester("TestUnsubscribe")
	if err != nil {
		t.Fatal(err)
	}
	// TODO: add tPoolTester.Close() method
	//	defer tpt.Close()

	// Check the transaction pool is empty when initialized.
	if len(tpt.tpool.transactionSets) != 0 {
		t.Fatal("transaction pool is not empty")
	}

	// Create a mock subscriber and subscribe it to the transaction pool.
	ms := mockSubscriber{}
	tpt.tpool.TransactionPoolSubscribe(&ms)
	if len(ms.txns) != 0 {
		t.Fatalf("mock subscriber has received %v transactions; shouldn't have received any yet", len(ms.txns))
	}

	// Create a valid transaction set and check that the mock subscriber's
	// transaction list is updated.
	_, err = tpt.wallet.SendSiacoins(types.NewCurrency64(100), types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tpt.tpool.transactionSets) != 1 {
		t.Error("sending coins didn't increase the transaction sets by 1")
	}
	numTxns := 0
	for _, txnSet := range tpt.tpool.transactionSets {
		numTxns += len(txnSet)
	}
	if len(ms.txns) != numTxns {
		t.Errorf("mock subscriber should've received %v transactions; received %v instead", numTxns, len(ms.txns))
	}
	
	numSubscribers := len(tpt.tpool.subscribers)
	tpt.tpool.Unsubscribe(&ms)
	if len(tpt.tpool.subscribers) != numSubscribers-1 {
		t.Error("transaction pool failed to unsubscribe mock subscriber")
	}
}

