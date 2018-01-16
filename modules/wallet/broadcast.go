package wallet

import (
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/bolt"
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

// markConfirmation is a helper function that sets a certain transactions to markConfirmation
// or unconfirmed. It also updates the state on disk.
func (bts *broadcastedTSet) markConfirmation(txid types.TransactionID, confirmed bool) error {
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

// rebroadcastOldTransaction rebroadcasts transactions that haven't been
// confirmed within rebroadcastInterval blocks
func (w *Wallet) rebroadcastOldTransactions(tx *bolt.Tx, cc modules.ConsensusChange) error {
	// Get the current consensus height
	consensusHeight, err := dbGetConsensusHeight(tx)
	if err != nil {
		return err
	}

	// Build an index to quickly map a transaction to a set in broadcastedTSets
	broadcastedTxns := make(map[types.TransactionID]modules.TransactionSetID)
	for tSetID, bts := range w.broadcastedTSets {
		for _, txn := range bts.transactions {
			broadcastedTxns[txn.ID()] = tSetID
		}
	}

	// Mark reverted transactions as not confirmed
	for _, block := range cc.RevertedBlocks {
		for _, txn := range block.Transactions {
			if tSetID, exists := broadcastedTxns[txn.ID()]; exists {
				bts := w.broadcastedTSets[tSetID]
				if err := bts.markConfirmation(txn.ID(), false); err != nil {
					return err
				}
			}
		}
	}

	// Mark applied transactions as confirmed
	for _, block := range cc.AppliedBlocks {
		for _, txn := range block.Transactions {
			if tSetID, exists := broadcastedTxns[txn.ID()]; exists {
				bts := w.broadcastedTSets[tSetID]
				if err := bts.markConfirmation(txn.ID(), true); err != nil {
					return err
				}
			}
		}
	}

	// Check if all transactions of the set are confirmed
	for tSetID, bts := range w.broadcastedTSets {
		confirmed := true
		for _, c := range bts.confirmedTxn {
			if !c {
				confirmed = false
				break
			}
		}
		// If the transaction set has been confirmed for one broadcast cycle it
		// should be safe to remove it
		if confirmed && consensusHeight >= bts.lastTry+RebroadcastInterval {
			if err := w.deleteBroadcastedTSet(tSetID); err != nil {
				return err
			}
			continue
		}
		// If the transaction set has been confirmed recently we wait a little
		// bit longer before we remove it
		if confirmed {
			continue
		}
		// If the transaction set is not confirmed and hasn't been broadcasted
		// for rebroadcastInterval blocks we try to broadcast it again
		if consensusHeight >= bts.lastTry+RebroadcastInterval {
			bts.lastTry = consensusHeight
			go func(tSet []types.Transaction) {
				if err := w.tpool.AcceptTransactionSet(tSet); err != nil {
					w.log.Println("WARNING: Rebroadcast failed: ", err)
				}
			}(bts.transactions)
			// Delete the transaction set once we have tried for RespendTimeout
			// blocks
			if consensusHeight >= bts.firstTry+RebroadcastTimeout {
				if err := w.deleteBroadcastedTSet(tSetID); err != nil {
					w.log.Println("ERROR: Failed to delete broadcasted TSet from db: ", err)
				}
			}
		}
	}
	return nil
}
