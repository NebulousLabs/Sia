package wallet

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	errOutOfBounds = errors.New("requesting transactions at unknown confirmation heights")
)

// AddressTransactions returns all of the wallet transactions associated with a
// single unlock hash.
func (w *Wallet) AddressTransactions(uh types.UnlockHash) (pts []modules.ProcessedTransaction, err error) {
	if err := w.tg.Add(); err != nil {
		return []modules.ProcessedTransaction{}, err
	}
	defer w.tg.Done()
	// ensure durability of reported transactions
	w.mu.Lock()
	defer w.mu.Unlock()
	if err = w.syncDB(); err != nil {
		return
	}

	txnIndices, _ := dbGetAddrTransactions(w.dbTx, uh)
	for _, i := range txnIndices {
		pt, err := dbGetProcessedTransaction(w.dbTx, i)
		if err != nil {
			continue
		}
		pts = append(pts, pt)
	}
	return pts, nil
}

// AddressUnconfirmedTransactions returns all of the unconfirmed wallet transactions
// related to a specific address.
func (w *Wallet) AddressUnconfirmedTransactions(uh types.UnlockHash) (pts []modules.ProcessedTransaction, err error) {
	if err := w.tg.Add(); err != nil {
		return []modules.ProcessedTransaction{}, err
	}
	defer w.tg.Done()
	// ensure durability of reported transactions
	w.mu.Lock()
	defer w.mu.Unlock()
	if err = w.syncDB(); err != nil {
		return
	}

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
	return pts, err
}

// Transaction returns the transaction with the given id. 'False' is returned
// if the transaction does not exist.
func (w *Wallet) Transaction(txid types.TransactionID) (pt modules.ProcessedTransaction, found bool, err error) {
	if err := w.tg.Add(); err != nil {
		return modules.ProcessedTransaction{}, false, err
	}
	defer w.tg.Done()
	// ensure durability of reported transaction
	w.mu.Lock()
	defer w.mu.Unlock()
	if err = w.syncDB(); err != nil {
		return
	}

	// Get the keyBytes for the given txid
	keyBytes, err := dbGetTransactionIndex(w.dbTx, txid)
	if err != nil {
		return modules.ProcessedTransaction{}, false, nil
	}

	// Retrieve the transaction
	found = encoding.Unmarshal(w.dbTx.Bucket(bucketProcessedTransactions).Get(keyBytes), &pt) == nil
	return
}

// Transactions returns all transactions relevant to the wallet that were
// confirmed in the range [startHeight, endHeight].
func (w *Wallet) Transactions(startHeight, endHeight types.BlockHeight) (pts []modules.ProcessedTransaction, err error) {
	if err := w.tg.Add(); err != nil {
		return nil, err
	}
	defer w.tg.Done()
	// ensure durability of reported transactions
	w.mu.Lock()
	defer w.mu.Unlock()
	if err = w.syncDB(); err != nil {
		return
	}

	height, err := dbGetConsensusHeight(w.dbTx)
	if err != nil {
		return
	} else if startHeight > height || startHeight > endHeight {
		return nil, errOutOfBounds
	}

	// Get the bucket, the largest key in it and the cursor
	bucket := w.dbTx.Bucket(bucketProcessedTransactions)
	cursor := bucket.Cursor()
	nextKey := bucket.Sequence() + 1

	// Database is empty
	if nextKey == 1 {
		return
	}

	var pt modules.ProcessedTransaction
	keyBytes := make([]byte, 8)
	var result int
	func() {
		// Recover from possible panic during binary search
		defer func() {
			r := recover()
			if r != nil {
				err = fmt.Errorf("%v", r)
			}
		}()

		// Start binary searching
		result = sort.Search(int(nextKey), func(i int) bool {
			// Create the key for the index
			binary.BigEndian.PutUint64(keyBytes, uint64(i))

			// Retrieve the processed transaction
			key, ptBytes := cursor.Seek(keyBytes)
			if build.DEBUG && key == nil {
				panic("Failed to retrieve processed Transaction by key")
			}

			// Decode the transaction
			if err = decodeProcessedTransaction(ptBytes, &pt); build.DEBUG && err != nil {
				panic(err)
			}

			return pt.ConfirmationHeight >= startHeight
		})
	}()
	if err != nil {
		return
	}

	if uint64(result) == nextKey {
		// No transaction was found
		return
	}

	// Create the key that corresponds to the result of the search
	binary.BigEndian.PutUint64(keyBytes, uint64(result))

	// Get the processed transaction and decode it
	key, ptBytes := cursor.Seek(keyBytes)
	if build.DEBUG && key == nil {
		build.Critical("Couldn't find the processed transaction from the search.")
	}
	if err = decodeProcessedTransaction(ptBytes, &pt); build.DEBUG && err != nil {
		build.Critical(err)
	}

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
func (w *Wallet) UnconfirmedTransactions() ([]modules.ProcessedTransaction, error) {
	if err := w.tg.Add(); err != nil {
		return nil, err
	}
	defer w.tg.Done()
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.unconfirmedProcessedTransactions, nil
}
