package wallet

import (
	"math/big"
)

// WalletInfo contains basic information about the wallet.
type WalletInfo struct {
	Balance      *big.Int
	FullBalance  *big.Int
	NumAddresses int
}

// Info fills out and returns a WalletInfo struct.
func (w *Wallet) Info() (status WalletInfo, err error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	status = WalletInfo{
		Balance:      w.Balance(false),
		FullBalance:  w.Balance(true),
		NumAddresses: len(w.keys),
	}

	return
}
