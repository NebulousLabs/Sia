package wallet

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// ProcessConsensusChange parses a consensus change to update the set of
// confirmed outputs known to the wallet.
func (w *Wallet) ProcessConsensusChange(cc modules.ConsensusChange) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	// Iterate through the output diffs (siacoin and siafund) and apply all of
	// them. Only apply the outputs that relate to unlock hashes we understand.
	for _, diff := range cc.SiacoinOutputDiffs {
		_, exists := w.SiacoinOutputs[diff.ID]
		if diff.Direction == modules.DiffApply {
			if exists && build.DEBUG {
				panic("adding an existing output to wallet")
			}
			w.siacoinOutputs[diff.ID] = diff.SiacoinOutput
		} else {
			if !exists && build.DEBUG {
				panic("deleting nonexisting output from wallet")
			}
			delete(w.siacoinOutputs, diff.ID)
		}
	}
	for _, diff := range cc.SiafundOutputDiffs {
		_, exists := w.SiafundOutputs[diff.ID]
		if diff.Direction == modules.DiffApply {
			if exists && build.DEBUG {
				panic("adding an existing output to wallet")
			}
			w.siafundOutputs[diff.ID] = diff.SiafundOutput
		} else {
			if !exists && build.DEBUG {
				panic("deleting nonexisting output from wallet")
			}
			delete(w.siafundOutputs, diff.ID)
		}
	}
	for _, diff := range cc.SiafundPoolDiffs {
		if diff.Direction == modules.DiffApply {
			w.siafundPool = diff.Adjusted
		} else {
			w.siafundPool = diff.Previous
		}
	}
}

// ReceiveUpdatedUnconfirmedTransactions updates the wallet's unconfirmed
// transaction set.
func (w *Wallet) ReceiveUpdatedUnconfirmedTransactions(txns []types.Transaction, _ modules.ConsensusChange) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)
	w.unconfirmedTransactions = txns
}
