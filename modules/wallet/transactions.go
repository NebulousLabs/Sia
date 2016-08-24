package wallet

import (
	"errors"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

var (
	errOutOfBounds = errors.New("requesting transactions at unknown confirmation heights")
)

// AddressTransactions returns all of the wallet transactions associated with a
// single unlock hash.
func (w *Wallet) AddressTransactions(uh types.UnlockHash) (pts []modules.ProcessedTransaction) {
	w.mu.Lock()
	defer w.mu.Unlock()

	err := w.db.View(func(tx *bolt.Tx) error {
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
	if err != nil {
		panic(err)
	}

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
	found := false
	w.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucketProcessedTransactions).Cursor()
		for key, val := c.First(); key != nil; key, val = c.Next() {
			if err := encoding.Unmarshal(val, &pt); err != nil {
				return err
			}
			if pt.TransactionID == txid {
				found = true
				break
			}
		}
		return nil
	})
	return pt, found
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
		c := tx.Bucket(bucketProcessedTransactions).Cursor()
		for key, val := c.First(); key != nil; key, val = c.Next() {
			var pt modules.ProcessedTransaction
			if err := encoding.Unmarshal(val, &pt); err != nil {
				return err
			}
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
		return nil
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
