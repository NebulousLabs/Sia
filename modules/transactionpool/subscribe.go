package transactionpool

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// updateSubscribersTransactions sends a new transaction pool update to all
// subscribers.
func (tp *TransactionPool) updateSubscribersTransactions() {
	var txns []types.Transaction
	var cc modules.ConsensusChange
	for _, tSet := range tp.transactionSets {
		txns = append(txns, tSet...)
	}
	for _, tSetDiff := range tp.transactionSetDiffs {
		cc = cc.Append(tSetDiff)
	}
	for _, subscriber := range tp.subscribers {
		subscriber.ReceiveUpdatedUnconfirmedTransactions(txns, cc)
	}
}

// TransactionPoolSubscribe adds a subscriber to the transaction pool.
// Subscribers will receive the full transaction set every time there is a
// signficant change to the transaction pool.
func (tp *TransactionPool) TransactionPoolSubscribe(subscriber modules.TransactionPoolSubscriber) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	// Add the subscriber to the subscriber list.
	tp.subscribers = append(tp.subscribers, subscriber)

	// Send the new subscriber the transaction pool set.
	var txns []types.Transaction
	for _, tSet := range tp.transactionSets {
		txns = append(txns, tSet...)
	}
	var cc modules.ConsensusChange
	for _, tSetDiff := range tp.transactionSetDiffs {
		cc = cc.Append(tSetDiff)
	}
	subscriber.ReceiveUpdatedUnconfirmedTransactions(txns, cc)
}
