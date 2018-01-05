package wallet

import "github.com/NebulousLabs/Sia/types"

// broadcastedTSet is a helper struct to keep track of transaction sets and to
// help rebroadcast them.
type broadcastedTSet struct {
	firstTry     types.BlockHeight            // first time the tSet was broadcasted
	lastTry      types.BlockHeight            // last time the tSet was broadcasted
	confirmedTxn map[types.TransactionID]bool // tracks confirmed txns of set
	transactions []types.Transaction          // the tSet
}

// newBroadcastedTSet creates a broadcastedTSet from a normal tSet
func (w *Wallet) newBroadcastedTSet(tSet []types.Transaction) (bts *broadcastedTSet, err error) {
	bts = &broadcastedTSet{}
	// Set the height of the first and last try
	bts.firstTry, err = dbGetConsensusHeight(w.dbTx)
	if err != nil {
		return
	}
	bts.lastTry = bts.firstTry

	// Initialize confirmedTxn and transactions
	bts.confirmedTxn = make(map[types.TransactionID]bool)
	for _, txn := range tSet {
		bts.confirmedTxn[txn.ID()] = false
		bts.transactions = append(bts.transactions, txn)
	}
	return
}
