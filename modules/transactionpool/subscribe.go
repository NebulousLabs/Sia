package transactionpool

import (
	"github.com/NebulousLabs/Sia/build"
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

// updateSubscribersConsensus sends a new consensus change to all subscribers.
func (tp *TransactionPool) updateSubscribersConsensus(cc modules.ConsensusChange) {
	for _, subscriber := range tp.subscribers {
		subscriber.ReceiveConsensusSetUpdate(cc)
	}
}

// TransactionPoolSubscribe adds a subscriber to the transaction pool.
// Subscribers will receive all consensus set changes as well as transaction
// pool changes, and should not subscribe to both.
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
