package transactionpool

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"
)

type sanityCheckSubscriber struct {
	transactionSets map[TransactionSetID][]types.Transaction
}

// newSanityCheckSubscriber returns a new sanityCheckSubscriber with no
// transaction sets.
func newSanityCheckSubscriber() *sanityCheckSubscriber {
	return &sanityCheckSubscriber{
		transactionSets: make(map[TransactionSetID][]types.Transaction),
	}
}

// ReceiveUpdatedUnconfirmedTransactions updates the sanityCheckSubscriber's
// transactionSets using the diff sent from the tpool. It is needed to satisfy
// the TransactionPoolSubscriber interface.
func (s *sanityCheckSubscriber) ReceiveUpdatedUnconfirmedTransactions(diff *modules.TransactionPoolDiff) {
	for _, setID := range diff.RevertedTransactions {
		delete(s.transactionSets, TransactionSetID(setID))
	}
	for _, unconfirmedTxnSet := range diff.AppliedTransactions {
		s.transactionSets[TransactionSetID(unconfirmedTxnSet.ID)] = unconfirmedTxnSet.Transactions
	}
}

// subscriberSanityCheck performs a sanity check on the transaction pool. It
// panics if the map of transaction sets in the subscriber's state is not
// exactly the same as the map of transaction sets in the transaction pool.
func (tp *TransactionPool) subscriberSanityCheck() {
	// 1/30 chance of running this check because it is expensive
	if fastrand.Intn(30) != 0 || !build.DEBUG {
		return
	}

	if len(tp.transactionSets) != len(tp.sanityCheck.transactionSets) {
		panic("length of tp transactions sets different from sanityCheckSubscriber's ")
	}

	for tpoolSetID, tpoolSet := range tp.transactionSets {
		subscriberSet, ok := tp.sanityCheck.transactionSets[tpoolSetID]
		if !ok {
			// Doesn't contain a set the tpool contains.
			panic("sanityCheckSubscriber doesn't contain same transaction set as tpool")
		}

		if len(tpoolSet) != len(subscriberSet) {
			panic("sanityCheckSubscriber txn set has different size than corresponding set in tpool")
		}
		// Check that both sets contain the exact same transactions

		tpoolTxns := make(map[types.TransactionID]struct{})
		for _, txn := range tpoolSet {
			tpoolTxns[txn.ID()] = struct{}{}
		}
		for _, txn := range subscriberSet {
			_, ok := tpoolTxns[txn.ID()]
			if !ok {
				// Doesn't contain a transacion the tpool contains.
				panic("sanityCheckSubscriber doesn't contain the same transaction in the same set as tpool")
			}
		}
	}
}
