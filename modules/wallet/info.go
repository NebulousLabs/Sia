package wallet

import (
	"github.com/NebulousLabs/Sia/modules"
)

// Info fills out and returns a WalletInfo struct.
func (w *Wallet) Info() modules.WalletInfo {
	counter := w.mu.RLock("wallet Info")
	defer w.mu.RUnlock("wallet Info", counter)

	return modules.WalletInfo{
		Balance:      w.Balance(false),
		FullBalance:  w.Balance(true),
		NumAddresses: len(w.keys),
	}
}
