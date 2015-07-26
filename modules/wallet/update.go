package wallet

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// applyDiff will take the output and either add or delete it from the set of
// outputs known to the wallet. If adding is true, then new outputs will be
// added and expired outputs will be deleted. If adding is false, then new
// outputs will be deleted and expired outputs will be added.
func (w *Wallet) applyDiff(scod modules.SiacoinOutputDiff, dir modules.DiffDirection) {
	// See if the output in the diff is known to the wallet.
	key, exists := w.keys[scod.SiacoinOutput.UnlockHash]
	if !exists {
		return
	}

	if scod.Direction == dir {
		// FundTransaction creates outputs and adds them immediately. They will
		// also show up from the transaction pool, occasionally causing
		// repeats. Additionally, outputs that used to exist are not deleted.
		// If they get re-added, we need to know the age of the output.
		output, exists := key.outputs[scod.ID]
		if exists {
			if !output.spendable {
				output.spendable = true
			}
			return
		}

		// Add the output. Age is set to 0 because the output has not been
		// spent yet.
		ko := &knownOutput{
			id:     scod.ID,
			output: scod.SiacoinOutput,

			spendable: true,
			age:       0,
		}
		key.outputs[scod.ID] = ko
	} else {
		if build.DEBUG {
			_, exists := key.outputs[scod.ID]
			if !exists {
				panic("trying to delete an output that doesn't exist?")
			}
		}
		key.outputs[scod.ID].spendable = false
	}
}

func (w *Wallet) ProcessConsensusChange(cc modules.ConsensusChange) {
	// TODO: Restruture whole wallet.

	// Adjust the confirmed set of diffs.
	for _, scod := range cc.SiacoinOutputDiffs {
		w.applyDiff(scod, modules.DiffApply)
	}

	// Update the wallet age and consensus height. Though they update together,
	// the wallet age can be altered/reset, but the consensus height cannot.
	w.age -= len(cc.RevertedBlocks)
	w.consensusHeight -= types.BlockHeight(len(cc.RevertedBlocks))
	w.age += len(cc.AppliedBlocks)
	w.consensusHeight += types.BlockHeight(len(cc.AppliedBlocks))

	// Update the siafund addresses.
	for _, diff := range cc.SiafundOutputDiffs {
		_, exists := w.siafundAddresses[diff.SiafundOutput.UnlockHash]
		if exists {
			w.applySiafundDiff(diff, modules.DiffApply)
		}
	}
}

// ReceiveTransactionPoolUpdate gets all of the changes in the confirmed and
// unconfirmed set and uses them to update the balance and transaction history
// of the wallet.
func (w *Wallet) ReceiveUpdatedUnconfirmedTransactions(_ []types.Transaction, unconfirmedCC modules.ConsensusChange) {
	// TODO: Restructure whole wallet.
	/*
		lockID := w.mu.Lock()
		defer w.mu.Unlock(lockID)

		// Remove all of the current unconfirmed diffs - they are being replaced
		// wholesale.
		for _, diff := range w.unconfirmedDiffs {
			w.applyDiff(diff, modules.DiffRevert)
		}

		// Add all of the unconfirmed diffs to the wallet.
		w.unconfirmedDiffs = unconfirmedSiacoinDiffs
		for _, diff := range w.unconfirmedDiffs {
			w.applyDiff(diff, modules.DiffApply)
		}
		if len(cc.SiafundPoolDiffs) > 0 {
			if cc.SiafundPoolDiffs[len(cc.SiafundPoolDiffs)-1].Direction == modules.DiffApply {
				w.siafundPool = cc.SiafundPoolDiffs[len(cc.SiafundPoolDiffs)-1].Adjusted
			} else {
				w.siafundPool = cc.SiafundPoolDiffs[len(cc.SiafundPoolDiffs)-1].Previous
			}
		}

	*/
}
