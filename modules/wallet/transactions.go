package wallet

import (
	"errors"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

var (
	errOutOfBounds      = errors.New("requesting transactions at unknown confirmation heights")
	errNoHistoryForAddr = errors.New("no history found for provided address")
)

// AddressTransactions returns all of the wallet transactions associated with a
// single unlock hash.
func (w *Wallet) AddressTransactions(uh types.UnlockHash) (pts []modules.ProcessedTransaction) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.db.View(func(tx *bolt.Tx) error {
		return dbForEachProcessedTransaction(tx, func(pt modules.ProcessedTransaction) {
			relevant := false
			for _, input := range pt.Inputs {
				relevant = relevant || input.RelatedAddress == uh
			}
			for _, output := range pt.Outputs {
				relevant = relevant || output.RelatedAddress == uh
			}
			if relevant {
				pts = append(pts, pt)
			}
		})
	})

	return pts
}

// AddressUnconfirmedHistory returns all of the unconfirmed wallet transactions
// related to a specific address.
func (w *Wallet) AddressUnconfirmedTransactions(uh types.UnlockHash) (pts []modules.ProcessedTransaction) {
	w.mu.Lock()
	defer w.mu.Unlock()

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
func (w *Wallet) Transaction(txid types.TransactionID) (modules.ProcessedTransaction, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	var pt modules.ProcessedTransaction
	err := w.db.View(func(tx *bolt.Tx) error {
		var err error
		pt, err = dbGetProcessedTransaction(tx, txid)
		return err
	})
	return pt, err == nil
}

// Transactions returns all transactions relevant to the wallet that were
// confirmed in the range [startHeight, endHeight].
func (w *Wallet) Transactions(startHeight, endHeight types.BlockHeight) (pts []modules.ProcessedTransaction, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if startHeight > w.consensusSetHeight || startHeight > endHeight {
		return nil, errOutOfBounds
	}

	err = w.db.View(func(tx *bolt.Tx) error {
		return dbForEachProcessedTransaction(tx, func(pt modules.ProcessedTransaction) {
			if startHeight <= pt.ConfirmationHeight && pt.ConfirmationHeight <= endHeight {
				pts = append(pts, pt)
			}
		})
	})
	return
}

// UnconfirmedTransactions returns the set of unconfirmed transactions that are
// relevant to the wallet.
func (w *Wallet) UnconfirmedTransactions() []modules.ProcessedTransaction {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.unconfirmedProcessedTransactions
}
