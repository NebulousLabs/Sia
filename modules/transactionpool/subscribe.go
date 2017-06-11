package transactionpool

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

// updateSubscribersTransactions sends a new transaction pool update to all
// subscribers.
func (tp *TransactionPool) updateSubscribersTransactions() {
	var diffs []*modules.TransactionSetDiff
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
		diffs = append(diffs, &modules.TransactionSetDiff{
			Direction: modules.DiffRevert,
			ID:        crypto.Hash(id),
		})
	}

	// Clear the subscriber sets map.
	for _, diff := range diffs {
		delete(tp.subscriberSets, TransactionSetID(diff.ID))
	}

	// Create all of the diffs for sets that have been recently created.
	for id, set := range tp.transactionSets {
		_, exists := tp.subscriberSets[id]
		if exists {
			// The transaction set has already been sent in an update.
			continue
		}

		// Report that this transaction set is new to the transaction pool.
		diff := &modules.TransactionSetDiff{
			Change:       tp.transactionSetDiffs[id],
			Direction:    modules.DiffApply,
			ID:           crypto.Hash(id),
			Transactions: set,
		}
		// Add this diff to our set of subscriber diffs.
		tp.subscriberSets[id] = diff
		diffs = append(diffs, diff)
	}

	for _, subscriber := range tp.subscribers {
		subscriber.ReceiveUpdatedUnconfirmedTransactions(diffs)
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
	diffs := make([]*modules.TransactionSetDiff, 0, len(tp.subscriberSets))
	for _, diff := range tp.subscriberSets {
		diffs = append(diffs, diff)
	}
	subscriber.ReceiveUpdatedUnconfirmedTransactions(diffs)
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
