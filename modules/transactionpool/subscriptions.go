package transactionpool

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
)

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
			revertTxns := tp.revertTxnsUpdates[i]
			applyTxns := tp.applyTxnsUpdates[i]
			tp.mu.RUnlock(id)
			subscriber.ReceiveTransactionPoolUpdate(revertTxns, revertBlocks, applyBlocks, applyTxns)
			i++
		}

		// Wait until there has been another update.
		<-update
	}
}

// updateSubscribers adds another entry to the update list and informs the
// update threads (via channels) that there's a new update to send.
func (tp *TransactionPool) updateSubscribers(revertedBlocks, appliedBlocks []consensus.Block, revertedTxns, appliedTxns []consensus.Transaction) {
	// Add the changes to the update set.
	tp.revertBlocksUpdates = append(tp.revertBlocksUpdates, revertedBlocks)
	tp.applyBlocksUpdates = append(tp.applyBlocksUpdates, appliedBlocks)
	tp.revertTxnsUpdates = append(tp.revertTxnsUpdates, revertedTxns)
	tp.applyTxnsUpdates = append(tp.applyTxnsUpdates, appliedTxns)

	for _, subscriber := range tp.subscribers {
		// If the channel is already full, don't block.
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

// Subscribe adds a subscriber to the transaction pool.
func (tp *TransactionPool) TransactionPoolSubscribe(subscriber modules.TransactionPoolSubscriber) {
	c := make(chan struct{}, 1)
	id := tp.mu.Lock()
	tp.subscribers = append(tp.subscribers, c)
	tp.mu.Unlock(id)
	go tp.threadedSendUpdates(c, subscriber)
}
