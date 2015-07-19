package wallet

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// applySiafundDiff applies a siafund diff to the wallet.
func (w *Wallet) applySiafundDiff(diff modules.SiafundOutputDiff, dir modules.DiffDirection) {
	if diff.Direction == dir {
		if build.DEBUG {
			_, exists := w.siafundOutputs[diff.ID]
			if exists {
				panic("applying a siafund output that exists")
			}
		}

		w.siafundOutputs[diff.ID] = diff.SiafundOutput
	} else {
		if build.DEBUG {
			_, exists := w.siafundOutputs[diff.ID]
			if !exists {
				panic("deleting a siafund output that doesn't exist")
			}
		}

		delete(w.siafundOutputs, diff.ID)
	}
}

// SiafundBalance returns the number of siafunds owned by the wallet, and the
// number of siacoins available through siafund claims.
func (w *Wallet) SiafundBalance() (siafundBalance types.Currency, siacoinClaimBalance types.Currency) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	// Add up the siafunds and siacoin claims in all the outputs known to the
	// wallet.
	for _, sfo := range w.siafundOutputs {
		siafundBalance = siafundBalance.Add(sfo.Value)
		siacoinsPerSiafund := w.siafundPool.Sub(sfo.ClaimStart).Div(types.SiafundCount)
		siacoinClaim := siacoinsPerSiafund.Mul(sfo.Value)
		siacoinClaimBalance = siacoinClaimBalance.Add(siacoinClaim)
	}
	return siafundBalance, siacoinClaimBalance
}
