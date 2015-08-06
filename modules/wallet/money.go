package wallet

import (
	"github.com/NebulousLabs/Sia/types"
)

// ConfirmedBalance returns the balance of the wallet according to all of the
// confirmed transactions.
func (w *Wallet) ConfirmedBalance() (siacoinBalance types.Currency, siafundBalance types.Currency, siafundClaimBalance types.Currency) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	for _, sco := range w.siacoinOutputs {
		siacoinBalance = siacoinBalance.Add(sco.Value)
	}
	for _, sfo := range w.siafundOutputs {
		siafundBalance = siafundBalance.Add(sfo.Value)
		siafundClaimBalance = siafundClaimBalance.Add(w.siafundPool.Sub(sfo.ClaimStart).Mul(sfo.Value))
	}
	return
}

// UnconfirmedBalance returns the number of outgoing and incoming siacoins in
// the unconfirmed transaction set. Refund outputs are included in this
// reporting.
func (w *Wallet) UnconfirmedBalance() (outgoingSiacoins types.Currency, incomingSiacoins types.Currency) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	for _, uwt := range w.unconfirmedWalletTransactions {
		if uwt.FundType == types.SpecifierSiacoinOutput {
			incomingSiacoins = incomingSiacoins.Add(uwt.Value)
		} else if uwt.FundType == types.SpecifierSiacoinInput {
			outgoingSiacoins = outgoingSiacoins.Add(uwt.Value)
		}
	}
	return
}

// NextAddress returns an unlock hash that is ready to recieve siacoins or
// siafunds. The address is generated using the primary address seed.
func (w *Wallet) NextAddress() (types.UnlockConditions, error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)
	return w.nextPrimarySeedAddress()
}
