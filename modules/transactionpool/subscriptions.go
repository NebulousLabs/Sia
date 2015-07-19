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
		updateCount := len(tp.unconfirmedTransactions)
		tp.mu.RUnlock(id)

		// Send each of the updates in order, starting from the first update
		// that has not yet been sent to the subscriber.
		for i < updateCount {
			var cc modules.ConsensusChange
			id := tp.mu.RLock()
			if tp.consensusChanges[i] != -1 {
				var err error
				cc, err = tp.consensusSet.ConsensusChange(tp.consensusChanges[i])
				if err != nil && build.DEBUG {
					panic("error when requesting consensus change from consensus set")
				}
			}
			unconfirmedTransactions := tp.unconfirmedTransactions[i]
			unconfirmedDiffs := tp.unconfirmedSiacoinDiffs[i]
			tp.mu.RUnlock(id)
			subscriber.ReceiveTransactionPoolUpdate(cc, unconfirmedTransactions, unconfirmedDiffs)
			i++
		}

		// Wait until there has been another update.
		<-update
	}
}

// updateSubscribers adds another entry to the update list and informs the
// update threads (via channels) that there's a new update to send.
func (tp *TransactionPool) updateSubscribers(cc modules.ConsensusChange, unconfirmedTransactions []types.Transaction, diffs []modules.SiacoinOutputDiff) {
	// Copy tp.unconfirmedTransactions to a separate memory slice -
	// tp.unconfirmedTransactions is constantly changing. cc should have
	// already been made safe by the consensus package.
	safeTxns := make([]types.Transaction, len(unconfirmedTransactions))
	safeDiffs := make([]modules.SiacoinOutputDiff, len(diffs))
	copy(safeTxns, unconfirmedTransactions)
	copy(safeDiffs, diffs)

	// If the consensus change variable is empty, add a -1, otherwise add the
	// prevConsensusIndex value.
	if len(cc.AppliedBlocks) == 0 {
		tp.consensusChanges = append(tp.consensusChanges, -1)
	} else {
		tp.consensusChanges = append(tp.consensusChanges, tp.consensusChangeIndex)
		tp.consensusChangeIndex++
	}
	tp.unconfirmedTransactions = append(tp.unconfirmedTransactions, safeTxns)
	tp.unconfirmedSiacoinDiffs = append(tp.unconfirmedSiacoinDiffs, safeDiffs)

	// Pass the update to every subscriber.
	for _, subscriber := range tp.subscribers {
		// If the channel is already full, don't block. This will prevent a
		// deadlocked subscriber from also deadlocking the transaction pool.
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

// TransactionPoolNotify adds a subscriber to the list who will be notified any
// time that there is a change to the transaction pool (new transaction or
// block), but that subscriber will not be told any details about the change.
func (tp *TransactionPool) TransactionPoolNotify() <-chan struct{} {
	c := make(chan struct{}, modules.NotifyBuffer)
	id := tp.mu.Lock()
	if len(tp.unconfirmedTransactions) != 0 {
		c <- struct{}{}
	}
	tp.subscribers = append(tp.subscribers, c)
	tp.mu.Unlock(id)
	return c
}

// TransactionPoolSubscribe adds a subscriber to the transaction pool that will
// be given a full list of changes via ReceiveTransactionPoolUpdate any time
// that there is a change.
func (tp *TransactionPool) TransactionPoolSubscribe(subscriber modules.TransactionPoolSubscriber) {
	c := make(chan struct{}, 1)
	id := tp.mu.Lock()
	tp.subscribers = append(tp.subscribers, c)
	tp.mu.Unlock(id)
	go tp.threadedSendUpdates(c, subscriber)
}
