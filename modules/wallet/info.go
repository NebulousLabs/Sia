package wallet

import (
	"github.com/NebulousLabs/Sia/modules"
)

// Info fills out and returns a WalletInfo struct.
func (w *Wallet) Info() modules.WalletInfo {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return modules.WalletInfo{
		Balance:      w.Balance(false),
		FullBalance:  w.Balance(true),
		NumAddresses: len(w.keys),
	}
}
