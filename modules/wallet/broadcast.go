package wallet

import (
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// broadcastedTSet is a helper struct to keep track of transaction sets and to
// help rebroadcast them.
type broadcastedTSet struct {
	firstTry     types.BlockHeight            // first time the tSet was broadcasted
	lastTry      types.BlockHeight            // last time the tSet was broadcasted
	confirmedTxn map[types.TransactionID]bool // tracks confirmed txns of set
	transactions []types.Transaction          // the tSet
	id           modules.TransactionSetID     // the tSet's ID
	w            *Wallet
}

// persistBTS is the on-disk version of the broadcastedTSets structure. This is
// necessary since we can't marshal a map directly. Instead we make sure that
// confirmedTxn[i] corresponds to the confirmation state of transactions[i].
type persistBTS struct {
	FirstTry     types.BlockHeight   // first time the tSet was broadcasted
	LastTry      types.BlockHeight   // last time the tSet was broadcasted
	ConfirmedTxn []bool              // tracks confirmed txns of set
	Transactions []types.Transaction // the tSet
}

// confirmed is a helper function that sets a certain transactions to confirmed
// or unconfirmed. It also updates the state on disk.
func (bts *broadcastedTSet) confirmed(txid types.TransactionID, confirmed bool) error {
	bts.confirmedTxn[txid] = confirmed
	return dbPutBroadcastedTSet(bts.w.dbTx, *bts)
}

// deleteBroadcastedTSet removes a broadcastedTSet from the wallet and disk
func (w *Wallet) deleteBroadcastedTSet(tSetID modules.TransactionSetID) error {
	// Remove it from wallet
	delete(w.broadcastedTSets, tSetID)

	// Remove it from disk
	if err := dbDeleteBroadcastedTSet(w.dbTx, tSetID); err != nil {
		return err
	}
	return nil
}

// newBroadcastedTSet creates a broadcastedTSet from a normal tSet
func (w *Wallet) newBroadcastedTSet(tSet []types.Transaction) (bts *broadcastedTSet, err error) {
	bts = &broadcastedTSet{
		w: w,
	}
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

	// Persist the new tSet
	bts.id = modules.TransactionSetID(crypto.HashAll(tSet))
	if err := dbPutBroadcastedTSet(w.dbTx, *bts); err != nil {
		return nil, err
	}
	return
}
