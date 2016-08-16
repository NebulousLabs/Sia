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

// AddressTransactions returns all of the wallet transactions associated with a
// single unlock hash.
func (w *Wallet) AddressTransactions(uh types.UnlockHash) (pts []modules.ProcessedTransaction) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, pt := range w.processedTransactions {
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

	for _, pt := range w.processedTransactions {
		if pt.TransactionID == txid {
			return pt, true
		}
	}
	return modules.ProcessedTransaction{}, false
}

// Transactions returns all transactions relevant to the wallet that were
// confirmed in the range [startHeight, endHeight].
func (w *Wallet) Transactions(startHeight, endHeight types.BlockHeight) (pts []modules.ProcessedTransaction, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if startHeight > w.consensusSetHeight || startHeight > endHeight {
		return nil, errOutOfBounds
	}
	if len(w.processedTransactions) == 0 {
		return nil, nil
	}

	for _, pt := range w.processedTransactions {
		if pt.ConfirmationHeight > endHeight {
			break
		}
		if pt.ConfirmationHeight >= startHeight {
			pts = append(pts, pt)
		}
	}
	return pts, nil
}

// UnconfirmedTransactions returns the set of unconfirmed transactions that are
// relevant to the wallet.
func (w *Wallet) UnconfirmedTransactions() []modules.ProcessedTransaction {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.unconfirmedProcessedTransactions
}
