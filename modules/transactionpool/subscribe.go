package transactionpool

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

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

func (tp *TransactionPool) updateSubscribersConsensus(cc modules.ConsensusChange) {
	for _, subscriber := range tp.subscribers {
		subscriber.ReceiveConsensusSetUpdate(cc)
	}
}

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
