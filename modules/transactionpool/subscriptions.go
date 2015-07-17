package transactionpool

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

func (tp *TransactionPool) sendNotification() {
	for _, subscriber := range tp.notifySubscribers {
		subscriber <- struct{}{}
	}
}

func (tp *TransactionPool) updateSubscribersTransactions() {
	var txns []types.Transaction
	for _, tSet := range tp.transactionSets {
		txns = append(txns, tSet...)
	}
	for _, subscriber := range tp.subscribers {
		subscriber.ReceiveUpdatedUnconfirmedTransactions(txns)
	}
	tp.sendNotification()
}

func (tp *TransactionPool) updateSubscribersConsensus(cc modules.ConsensusChange) {
	for _, subscriber := range tp.subscribers {
		subscriber.ReceiveConsensusSetUpdate(cc)
	}
	tp.sendNotification()
}

// TransactionPoolNotify adds a subscriber to the list who will be notified any
// time that there is a change to the transaction pool (new transaction or
// block), but that subscriber will not be told any details about the change.
func (tp *TransactionPool) TransactionPoolNotify() <-chan struct{} {
	c := make(chan struct{}, modules.NotifyBuffer)
	id := tp.mu.Lock()
	c <- struct{}{}
	tp.notifySubscribers = append(tp.notifySubscribers, c)
	tp.mu.Unlock(id)
	return c
}

// TransactionPoolSubscribe adds a subscriber to the transaction pool that will
// be given a full list of changes via ReceiveTransactionPoolUpdate any time
// that there is a change.
func (tp *TransactionPool) TransactionPoolSubscribe(subscriber modules.TransactionPoolSubscriber) {
	id := tp.mu.Lock()
	tp.subscribers = append(tp.subscribers, subscriber)
	for i := 0; i <= tp.consensusChangeIndex; i++ {
		cc, err := tp.consensusSet.ConsensusChange(i)
		if err != nil && build.DEBUG {
			panic(err)
		}
		subscriber.ReceiveConsensusSetUpdate(cc)
	}
	tp.mu.Unlock(id)
}
