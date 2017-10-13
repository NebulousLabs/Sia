package wallet

import (
	"encoding/binary"
	"errors"
	"sort"

	"github.com/NebulousLabs/Sia/build"
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

	it := dbProcessedTransactionsIterator(w.dbTx)
	for it.next() {
		pt := it.value()
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

	// Get the bucket, the largest key in it and the cursor
	bucket := w.dbTx.Bucket(bucketProcessedTransactions)
	cursor := bucket.Cursor()
	lastKey := bucket.Sequence()

	var pt modules.ProcessedTransaction
	keyBytes := make([]byte, 8)
	func() {
		// Start binary searching
		sort.Search(int(lastKey), func(i int) bool {
			// Create the key for the index
			binary.BigEndian.PutUint64(keyBytes, uint64(i))

			// Retrieve the processed transaction
			key, ptBytes := cursor.Seek(keyBytes)
			if build.DEBUG && key == nil {
				err = errors.New("Failed to retrieve processed Transaction by key")
			}

			// Decode the transaction
			if err := decodeProcessedTransaction(ptBytes, &pt); build.DEBUG && err != nil {
				err = errors.New("Failed to decode the processed transaction")
			}

			return pt.ConfirmationHeight >= startHeight
		})
	}()

	// Gather all transactions until endHeight is reached
	for pt.ConfirmationHeight <= endHeight {
		if build.DEBUG && pt.ConfirmationHeight < startHeight {
			build.Critical("wallet processed transactions are not sorted")
		}
		pts = append(pts, pt)

		// Get next processed transaction
		key, ptBytes := cursor.Next()
		if key == nil {
			break
		}

		// Decode the transaction
		if err := decodeProcessedTransaction(ptBytes, &pt); build.DEBUG && err != nil {
			panic("Failed to decode the processed transaction")
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
