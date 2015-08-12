package wallet

import (
	"errors"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	errOutOfBounds      = errors.New("requesting transactions at unknown confirmation heights")
	errNoHistoryForAddr = errors.New("no history found for provided address")
)

// History returns all of the confirmed transactions between 'startHeight' and
// 'endHeight' (inclusive).
func (w *Wallet) History(startHeight types.BlockHeight, endHeight types.BlockHeight) ([]modules.WalletTransaction, error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	if startHeight > w.consensusSetHeight || startHeight > endHeight {
		return nil, errOutOfBounds
	}
	if len(w.walletTransactions) == 0 {
		return nil, nil
	}

	var start, end int
	for start = 0; start < len(w.walletTransactions); start++ {
		if w.walletTransactions[start].ConfirmationHeight >= startHeight {
			break
		}
	}
	for end = start; end < len(w.walletTransactions); end++ {
		if w.walletTransactions[end].ConfirmationHeight > endHeight {
			break
		}
	}
	return w.walletTransactions[start:end], nil
}

// AddressHistory returns all of the wallet transactions associated with a
// single unlock hash.
func (w *Wallet) AddressHistory(uh types.UnlockHash) (wts []modules.WalletTransaction, err error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	_, exists := w.keys[uh]
	if !exists {
		return nil, errors.New("address not recognized by the wallet")
	}

	for _, wt := range w.walletTransactions {
		if wt.RelatedAddress == uh {
			wts = append(wts, wt)
		}
	}
	if len(wts) == 0 {
		return nil, errNoHistoryForAddr
	}
	return wts, nil
}

// UnconfirmedHistory returns the set of unconfirmed wallet transactions.
func (w *Wallet) UnconfirmedHistory() []modules.WalletTransaction {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)
	return w.unconfirmedWalletTransactions
}

// AddressUnconfirmedHistory returns all of the unconfirmed wallet transactions
// related to a specific address.
func (w *Wallet) AddressUnconfirmedHistory(uh types.UnlockHash) (wts []modules.WalletTransaction) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	for _, wt := range w.unconfirmedWalletTransactions {
		if wt.RelatedAddress == uh {
			wts = append(wts, wt)
		}
	}
	return wts
}

// Transaction returns the transaction with the given id. 'False' is returned
// if the transaction does not exist.
func (w *Wallet) Transaction(txid types.TransactionID) (txn types.Transaction, ok bool) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)
	txn, ok = w.transactions[txid]
	return txn, ok
}

// Transactions returns all transactions relevant to the wallet that were
// confirmed in the range [startHeight, endHeight].
func (w *Wallet) Transactions(startHeight, endHeight types.BlockHeight) (txns []types.Transaction, err error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	if startHeight > w.consensusSetHeight || startHeight > endHeight {
		return nil, errOutOfBounds
	}
	if len(w.walletTransactions) == 0 {
		return nil, nil
	}

	// prevTxid is kept because multiple WalletTransactions can be created from
	// the same source types.Transaction, and will appear in the slice
	// consecutively. This is an effective way to prevent duplicates from
	// appearing in the output.
	var prevTxid types.TransactionID
	for _, wt := range w.walletTransactions {
		if wt.ConfirmationHeight > endHeight {
			break
		}
		if wt.ConfirmationHeight >= startHeight && wt.TransactionID != prevTxid {
			prevTxid = wt.TransactionID
			txns = append(txns, w.transactions[wt.TransactionID])
		}
	}
	return txns, nil
}

// UnconfirmedTransactions returns the set of unconfirmed transactions that are
// relevant to the wallet.
func (w *Wallet) UnconfirmedTransactions() []types.Transaction {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)
	return w.unconfirmedTransactions
}
