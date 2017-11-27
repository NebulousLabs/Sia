package wallet

import (
	"errors"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	errOutOfBounds = errors.New("requesting transactions at unknown confirmation heights")
)

// AddressTransactions returns all of the wallet transactions associated with a
// single unlock hash.
func (w *Wallet) AddressTransactions(uh types.UnlockHash) (pts []modules.ProcessedTransaction) {
	// ensure durability of reported transactions
	w.mu.Lock()
	defer w.mu.Unlock()
	w.syncDB()

	txnIndices, _ := dbGetAddrTransactions(w.dbTx, uh)
	for _, i := range txnIndices {
		pt, err := dbGetProcessedTransaction(w.dbTx, i)
		if err != nil {
			continue
		}
		pts = append(pts, pt)
	}
	return pts
}

// AddressUnconfirmedHistory returns all of the unconfirmed wallet transactions
// related to a specific address.
func (w *Wallet) AddressUnconfirmedTransactions(uh types.UnlockHash) (pts []modules.ProcessedTransaction) {
	// ensure durability of reported transactions
	w.mu.Lock()
	defer w.mu.Unlock()
	w.syncDB()

	// Scan the full list of unconfirmed transactions to see if there are any
	// related transactions.
	for _, pt := range w.unconfirmedProcessedTransactions {
		relevant := false
		for _, input := range pt.Inputs {
			if input.RelatedAddress == uh {
				relevant = true
				break
			}
		}
		for _, output := range pt.Outputs {
			if output.RelatedAddress == uh {
				relevant = true
				break
			}
		}
		if relevant {
			pts = append(pts, pt)
		}
	}
	return pts
}

// Transaction returns the transaction with the given id. 'False' is returned
// if the transaction does not exist.
func (w *Wallet) Transaction(txid types.TransactionID) (pt modules.ProcessedTransaction, found bool) {
	// ensure durability of reported transaction
	w.mu.Lock()
	defer w.mu.Unlock()
	w.syncDB()

	it := dbProcessedTransactionsIterator(w.dbTx)
	for it.next() {
		pt := it.value()
		if pt.TransactionID == txid {
			return pt, true
		}
	}
	return modules.ProcessedTransaction{}, false
}

// Transactions returns all transactions relevant to the wallet that were
// confirmed in the range [startHeight, endHeight].
func (w *Wallet) Transactions(startHeight, endHeight types.BlockHeight) (pts []modules.ProcessedTransaction, err error) {
	// ensure durability of reported transactions
	w.mu.Lock()
	defer w.mu.Unlock()
	w.syncDB()

	height, err := dbGetConsensusHeight(w.dbTx)
	if err != nil {
		return
	} else if startHeight > height || startHeight > endHeight {
		return nil, errOutOfBounds
	}

	it := dbProcessedTransactionsIterator(w.dbTx)
	for it.next() {
		pt := it.value()
		if pt.ConfirmationHeight < startHeight {
			continue
		} else if pt.ConfirmationHeight > endHeight {
			// transactions are stored in chronological order, so we can
			// break as soon as we are above endHeight
			break
		} else {
			pts = append(pts, pt)
		}
	}
	return
}

// UnconfirmedTransactions returns the set of unconfirmed transactions that are
// relevant to the wallet.
func (w *Wallet) UnconfirmedTransactions() []modules.ProcessedTransaction {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.unconfirmedProcessedTransactions
}
