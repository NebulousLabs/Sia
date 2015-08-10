package wallet

import (
	"errors"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	errOutOfBounds = errors.New("requesting transactions at unknown confirmation heights")
)

// TransactionHistory returns all of the confirmed transactions between
// 'startBlock' and 'endBlock' (inclusive).
func (w *Wallet) TransactionHistory(startBlock types.BlockHeight, endBlock types.BlockHeight) ([]modules.WalletTransaction, error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	if startBlock > w.consensusSetHeight || startBlock > endBlock {
		return nil, errOutOfBounds
	}
	if len(w.walletTransactions) == 0 {
		return nil, nil
	}

	var start, end int
	for start = 0; start < len(w.walletTransactions); start++ {
		if w.walletTransactions[start].ConfirmationHeight >= startBlock {
			break
		}
	}
	for end = start; end < len(w.walletTransactions); end++ {
		if w.walletTransactions[end].ConfirmationHeight > endBlock {
			break
		}
	}
	return w.walletTransactions[start:end], nil
}

// AddressTransactionHistory returns all of the wallet transactions associated
// with a single unlock hash.
func (w *Wallet) AddressTransactionHistory(uh types.UnlockHash) (wts []modules.WalletTransaction, err error) {
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
		return nil, errors.New("no history found for provided addresse")
	}
	return wts, nil
}

// UnconfirmedTransactions returns the set of unconfirmed wallet transactions.
func (w *Wallet) UnconfirmedTransactions() []modules.WalletTransaction {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)
	return w.unconfirmedWalletTransactions
}

// AddressUnconfirmedTransactions returns all of the unconfirmed wallet
// transactions related to a specific address.
func (w *Wallet) AddressUnconfirmedTransactions(uh types.UnlockHash) (wts []modules.WalletTransaction) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	for _, wt := range w.unconfirmedWalletTransactions {
		if wt.RelatedAddress == uh {
			wts = append(wts, wt)
		}
	}
	return wts
}
