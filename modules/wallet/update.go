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

// ReceiveTransactionPoolUpdate gets all of the changes in the confirmed and
// unconfirmed set and uses them to update the balance and transaction history
// of the wallet.
func (w *Wallet) ReceiveTransactionPoolUpdate(revertedBlocks, appliedBlocks []types.Block, _ []types.Transaction, unconfirmedSiacoinDiffs []modules.SiacoinOutputDiff) {
	id := w.mu.Lock()
	defer w.mu.Unlock(id)

	for _, diff := range w.unconfirmedDiffs {
		w.applyDiff(diff, modules.DiffRevert)
	}

	for _, block := range revertedBlocks {
		w.age--

		scods, err := w.state.BlockOutputDiffs(block.ID())
		if err != nil {
			if build.DEBUG {
				panic(err)
			}
			continue
		}
		for _, scod := range scods {
			w.applyDiff(scod, modules.DiffRevert)
		}
	}
	for _, block := range appliedBlocks {
		w.age++

		scods, err := w.state.BlockOutputDiffs(block.ID())
		if err != nil {
			if build.DEBUG {
				panic(err)
			}
			continue
		}
		for _, scod := range scods {
			w.applyDiff(scod, modules.DiffApply)
		}
	}

	w.unconfirmedDiffs = unconfirmedSiacoinDiffs
	for _, diff := range w.unconfirmedDiffs {
		w.applyDiff(diff, modules.DiffApply)
	}

	w.notifySubscribers()
}
