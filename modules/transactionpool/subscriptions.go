package transactionpool

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
)

// UnconfirmedSiacoinOutputDiffs returns the set of siacoin output diffs that
// would be created immediately if all of the unconfirmed transactions were
// added to the blockchain.
func (tp *TransactionPool) unconfirmedSiacoinOutputDiffs() (scods []consensus.SiacoinOutputDiff) {
	// Grab the diffs by iterating through the transactions in the transaction
	// pool in order and grabbing the siacoin diffs that would be created by
	// each.
	currentTxn := tp.head
	for currentTxn != nil {
		// Produce diffs for the siacoin outputs consumed by this transaction.
		txn := currentTxn.transaction
		for _, input := range txn.SiacoinInputs {
			// Grab the output for the diff.
			output, exists := tp.siacoinOutputs[input.ParentID]
			if !exists {
				output, exists = tp.referenceSiacoinOutputs[input.ParentID]
				if consensus.DEBUG {
					if !exists {
						panic("could not find siacoin output")
					}
				}
			}

			scod := consensus.SiacoinOutputDiff{
				Direction:     consensus.DiffRevert,
				ID:            input.ParentID,
				SiacoinOutput: output,
			}
			scods = append(scods, scod)
		}

		// Produce diffs for the siacoin outputs created by this transaction.
		for i, output := range txn.SiacoinOutputs {
			scod := consensus.SiacoinOutputDiff{
				Direction:     consensus.DiffApply,
				ID:            txn.SiacoinOutputID(i),
				SiacoinOutput: output,
			}
			scods = append(scods, scod)
		}

		currentTxn = currentTxn.next
	}

	return
}

// threadedSendUpdates sends updates to a specific subscriber as they become
// available. Greater information can be found in consensus/subscribers.go
func (tp *TransactionPool) threadedSendUpdates(update chan struct{}, subscriber modules.TransactionPoolSubscriber) {
	i := 0
	for {
		id := tp.mu.RLock()
		updateCount := len(tp.revertBlocksUpdates)
		tp.mu.RUnlock(id)
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
func (tp *TransactionPool) updateSubscribers(revertedBlocks, appliedBlocks []consensus.Block, unconfirmedTransactions []consensus.Transaction, diffs []consensus.SiacoinOutputDiff) {
	// Add the changes to the update set.
	tp.revertBlocksUpdates = append(tp.revertBlocksUpdates, revertedBlocks)
	tp.applyBlocksUpdates = append(tp.applyBlocksUpdates, appliedBlocks)
	tp.unconfirmedTransactions = append(tp.unconfirmedTransactions, unconfirmedTransactions)
	tp.unconfirmedSiacoinDiffs = append(tp.unconfirmedSiacoinDiffs, diffs)

	for _, subscriber := range tp.subscribers {
		// If the channel is already full, don't block.
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
