package transactionpool

import (
	"gitlab.com/NebulousLabs/Sia/build"
	"gitlab.com/NebulousLabs/Sia/encoding"
	"gitlab.com/NebulousLabs/Sia/modules"
	"gitlab.com/NebulousLabs/Sia/types"
)

// updateSubscribersTransactions sends a new transaction pool update to all
// subscribers.
func (tp *TransactionPool) updateSubscribersTransactions() {
	diff := new(modules.TransactionPoolDiff)
	// Create all of the diffs for reverted sets.
	for id := range tp.subscriberSets {
		// The transaction set is still in the transaction pool, no need to
		// create an update.
		_, exists := tp.transactionSets[id]
		if exists {
			continue
		}

		// Report that this set has been removed. Negative diffs don't have all
		// fields filled out.
		diff.RevertedTransactions = append(diff.RevertedTransactions, modules.TransactionSetID(id))
	}

	// Clear the subscriber sets map.
	for _, revert := range diff.RevertedTransactions {
		delete(tp.subscriberSets, TransactionSetID(revert))
	}

	// Create all of the diffs for sets that have been recently created.
	for id, set := range tp.transactionSets {
		_, exists := tp.subscriberSets[id]
		if exists {
			// The transaction set has already been sent in an update.
			continue
		}

		// Report that this transaction set is new to the transaction pool.
		ids := make([]types.TransactionID, 0, len(set))
		sizes := make([]uint64, 0, len(set))
		for i := range set {
			encodedTxn := encoding.Marshal(set[i])
			sizes = append(sizes, uint64(len(encodedTxn)))
			ids = append(ids, set[i].ID())
		}
		ut := &modules.UnconfirmedTransactionSet{
			Change: tp.transactionSetDiffs[id],
			ID:     modules.TransactionSetID(id),

			IDs:          ids,
			Sizes:        sizes,
			Transactions: set,
		}
		// Add this diff to our set of subscriber diffs.
		tp.subscriberSets[id] = ut
		diff.AppliedTransactions = append(diff.AppliedTransactions, ut)
	}

	for _, subscriber := range tp.subscribers {
		subscriber.ReceiveUpdatedUnconfirmedTransactions(diff)
	}
}

// TransactionPoolSubscribe adds a subscriber to the transaction pool.
// Subscribers will receive the full transaction set every time there is a
// significant change to the transaction pool.
func (tp *TransactionPool) TransactionPoolSubscribe(subscriber modules.TransactionPoolSubscriber) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	// Check that this subscriber is not already subscribed.
	for _, s := range tp.subscribers {
		if s == subscriber {
			build.Critical("refusing to double-subscribe subscriber")
		}
	}

	// Add the subscriber to the subscriber list.
	tp.subscribers = append(tp.subscribers, subscriber)

	// Send the new subscriber the transaction pool set.
	diff := new(modules.TransactionPoolDiff)
	diff.AppliedTransactions = make([]*modules.UnconfirmedTransactionSet, 0, len(tp.subscriberSets))
	for _, ut := range tp.subscriberSets {
		diff.AppliedTransactions = append(diff.AppliedTransactions, ut)
	}
	subscriber.ReceiveUpdatedUnconfirmedTransactions(diff)
}

// Unsubscribe removes a subscriber from the transaction pool. If the
// subscriber is not in tp.subscribers, Unsubscribe does nothing. If the
// subscriber occurs more than once in tp.subscribers, only the earliest
// occurrence is removed (unsubscription fails).
func (tp *TransactionPool) Unsubscribe(subscriber modules.TransactionPoolSubscriber) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	// Search for and remove subscriber from list of subscribers.
	for i := range tp.subscribers {
		if tp.subscribers[i] == subscriber {
			tp.subscribers = append(tp.subscribers[0:i], tp.subscribers[i+1:]...)
			break
		}
	}
}
