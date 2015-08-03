package wallet

import (
	"sort"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// Info fills out and returns a WalletInfo struct.
func (w *Wallet) Info() modules.WalletInfo {
	wi := modules.WalletInfo{
		Balance:          w.Balance(false),
		FullBalance:      w.Balance(true),
		VisibleAddresses: []types.UnlockHash{},
	}

	counter := w.mu.RLock()
	wi.NumAddresses = len(w.keys)
	w.mu.RUnlock(counter)

	var sortingSpace crypto.HashSlice
	for va := range w.visibleAddresses {
		sortingSpace = append(sortingSpace, crypto.Hash(va))
	}
	sort.Sort(sortingSpace)
	for _, va := range sortingSpace {
		wi.VisibleAddresses = append(wi.VisibleAddresses, types.UnlockHash(va))
	}
	return wi
}
