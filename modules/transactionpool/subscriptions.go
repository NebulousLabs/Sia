package transactionpool

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// subscriptions.go manages subscriptions to the transaction pool. Every time
// there is a change in the transaction pool, subscribers are sent info about
// the changes, in the form of all of the blocks that have been applied or
// reverted in the consensus set, the current set of unconfirmed transactions,
// and the set of siacoin output diffs that result from the unconfirmed
// transactions.
//
// Subscriptions are set up to isolate the transaction pool from problems that
// occur with the subscriber. Each subscriber gets a gothread that calls
// 'update' in the correct order. If a subscriber crashes or deadlocks, the
// transcation pool will be unaffected.

// unconfirmedSiacoinOutputDiffs returns the set of siacoin output diffs that
// are created by the unconfirmed set of transactions.
func (tp *TransactionPool) unconfirmedSiacoinOutputDiffs() (scods []modules.SiacoinOutputDiff) {
	// Iterate through the unconfirmed transactions in order and record the
	// siacoin output diffs created.
	for _, txn := range tp.transactionList {
		// Produce diffs for the siacoin outputs consumed by this transaction.
		for _, input := range txn.SiacoinInputs {
			// Grab the output from the unconfirmed or reference set.
			output, exists := tp.siacoinOutputs[input.ParentID]
			if !exists {
				output, exists = tp.referenceSiacoinOutputs[input.ParentID]
				// Sanity check - output should exist in either the unconfirmed
				// or reference set.
				if build.DEBUG {
					if !exists {
						panic("could not find siacoin output")
					}
				}
			}

			scod := modules.SiacoinOutputDiff{
				Direction:     modules.DiffRevert,
				ID:            input.ParentID,
				SiacoinOutput: output,
			}
			scods = append(scods, scod)
		}

		// Produce diffs for the siacoin outputs created by this transaction.
		for i, output := range txn.SiacoinOutputs {
			scod := modules.SiacoinOutputDiff{
				Direction:     modules.DiffApply,
				ID:            txn.SiacoinOutputID(i),
				SiacoinOutput: output,
			}
			scods = append(scods, scod)
		}
	}

	return
}

// threadedSendUpdates sends updates to a specific subscriber as updates become
// available. If the subscriber deadlocks, this thread will deadlock, however
// that will not affect any of the other threads in the transaction pool.
func (tp *TransactionPool) threadedSendUpdates(update chan struct{}, subscriber modules.TransactionPoolSubscriber) {
	// Updates must be sent in order. This is achieved by having all of the
	// updates stored in the transaction pool in a specific order, and then
	// making blocking calls to 'ReceiveTransactionPoolUpates' until all of the
	// updates have been sent.
	i := 0
	for {
		// Determine how many total updates there are to send.
		id := tp.mu.RLock()
		updateCount := len(tp.revertBlocksUpdates)
		tp.mu.RUnlock(id)

		// Send each of the updates in order, starting from the first update
		// that has not yet been sent to the subscriber.
		for i < updateCount {
			id := tp.mu.RLock()
			revertBlocks := tp.revertBlocksUpdates[i]
			applyBlocks := tp.applyBlocksUpdates[i]
			unconfirmedTransactions := tp.unconfirmedTransactions[i]
			unconfirmedDiffs := tp.unconfirmedSiacoinDiffs[i]
			tp.mu.RUnlock(id)
			subscriber.ReceiveTransactionPoolUpdate(revertBlocks, applyBlocks, unconfirmedTransactions, unconfirmedDiffs)
			i++
		}

		// Wait until there has been another update.
		<-update
	}
}

// updateSubscribers adds another entry to the update list and informs the
// update threads (via channels) that there's a new update to send.
func (tp *TransactionPool) updateSubscribers(revertedBlocks, appliedBlocks []types.Block, unconfirmedTransactions []types.Transaction, diffs []modules.SiacoinOutputDiff) {
	// Add the changes to the update set.
	tp.revertBlocksUpdates = append(tp.revertBlocksUpdates, revertedBlocks)
	tp.applyBlocksUpdates = append(tp.applyBlocksUpdates, appliedBlocks)
	tp.unconfirmedTransactions = append(tp.unconfirmedTransactions, unconfirmedTransactions)
	tp.unconfirmedSiacoinDiffs = append(tp.unconfirmedSiacoinDiffs, diffs)

	// Notify every subscriber.
	for _, subscriber := range tp.subscribers {
		// If the channel is already full, don't block. This will prevent a
		// deadlocked subscriber from also deadlocking the transaction pool.
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

// TransactionPoolSubscribe adds a subscriber to the transaction pool.
func (tp *TransactionPool) TransactionPoolSubscribe(subscriber modules.TransactionPoolSubscriber) {
	c := make(chan struct{}, 1)
	id := tp.mu.Lock()
	tp.subscribers = append(tp.subscribers, c)
	tp.mu.Unlock(id)
	go tp.threadedSendUpdates(c, subscriber)
}
