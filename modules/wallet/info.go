package wallet

import (
	"github.com/NebulousLabs/Sia/modules"
)

// Info fills out and returns a WalletInfo struct.
func (w *Wallet) Info() modules.WalletInfo {
	wi := modules.WalletInfo{
		Balance:     w.Balance(false),
		FullBalance: w.Balance(true),
	}

	counter := w.mu.RLock()
	wi.NumAddresses = len(w.keys)
	w.mu.RUnlock(counter)

	for va := range w.visibleAddresses {
		wi.VisibleAddresses = append(wi.VisibleAddresses, va)
	}
	return wi
}
