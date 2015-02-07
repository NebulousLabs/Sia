package wallet

import (
	"github.com/NebulousLabs/Sia/consensus"
)

// applyDiff will take the output and either add or delete it from the set of
// outputs known to the wallet. If adding is true, then new outputs will be
// added and expired outputs will be deleted. If adding is false, then new
// outputs will be deleted and expired outputs will be added.
func (w *Wallet) applyDiff(scod consensus.SiacoinOutputDiff, adding bool) {
	isNew := scod.New
	if !adding {
		isNew = !isNew
	}

	// See if the output in the diff is known to the wallet.
	key, exists := w.keys[scod.SiacoinOutput.UnlockHash]
	if !exists {
		return
	}

	// Add the output if `isNew` is set, remove it otherwise.
	if isNew {
		output, exists := key.outputs[scod.ID]
		if exists {
			if consensus.DEBUG {
				if output.spendable {
					panic("output is market as spendable, but it's new?")
				}
			}
			output.spendable = true
		} else {
			ko := &knownOutput{
				id:        scod.ID,
				output:    scod.SiacoinOutput,
				spendable: true,
			}
			key.outputs[scod.ID] = ko
		}
	} else {
		output, exists := key.outputs[scod.ID]
		if consensus.DEBUG {
			if !exists {
				panic("trying to delete an output that doesn't exist?")
			}
		}

		output.spendable = false
	}
}

func (w *Wallet) update() error {
	w.state.RLock()
	defer w.state.RUnlock()

	// Remove all of the diffs that have been applied by the unconfirmed set of
	// transactions.
	for _, scod := range w.unconfirmedDiffs {
		w.applyDiff(scod, false)
	}

	// Apply the diffs in the state that have happened since the last update.
	removedBlocks, addedBlocks, err := w.state.BlocksSince(w.recentBlock)
	if err != nil {
		return err
	}
	for _, id := range removedBlocks {
		scods, err := w.state.BlockOutputDiffs(id)
		if err != nil {
			return err
		}
		for _, scod := range scods {
			w.applyDiff(scod, false)
		}
	}
	for _, id := range addedBlocks {
		scods, err := w.state.BlockOutputDiffs(id)
		if err != nil {
			return err
		}
		for _, scod := range scods {
			w.applyDiff(scod, true)
		}
	}

	// Get, apply, and store the unconfirmed diffs currently available in the transaction pool.
	w.unconfirmedDiffs = w.tpool.OutputDiffs()
	for _, scod := range w.unconfirmedDiffs {
		w.applyDiff(scod, true)
	}

	w.recentBlock = w.state.CurrentBlock().ID()

	return nil
}
