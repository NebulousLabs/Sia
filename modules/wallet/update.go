package wallet

import (
	"github.com/NebulousLabs/Sia/consensus"
)

// applyDiff will take the output and either add or delete it from the set of
// outputs known to the wallet. If adding is true, then new outputs will be
// added and expired outputs will be deleted. If adding is false, then new
// outputs will be deleted and expired outputs will be added.
func (w *Wallet) applyDiff(diff consensus.OutputDiff, adding bool) {
	isNew := diff.New
	if !adding {
		isNew = !isNew
	}

	// See if the output in the diff is known to the wallet.
	key, exists := w.keys[diff.Output.SpendHash]
	if !exists {
		return
	}

	// Add the output if `isNew` is set, remove it otherwise.
	if isNew {
		output, exists := key.outputs[diff.ID]
		if exists {
			if consensus.DEBUG {
				if output.spendable {
					panic("output is market as spendable, but it's new?")
				}
			}
			output.spendable = true
		} else {
			ko := &knownOutput{
				id:        diff.ID,
				output:    diff.Output,
				spendable: true,
			}
			key.outputs[diff.ID] = ko
		}
	} else {
		output, exists := key.outputs[diff.ID]
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
	for _, diff := range w.unconfirmedDiffs {
		w.applyDiff(diff, false)
	}

	// Apply the diffs in the state that have happened since the last update.
	removedBlocks, addedBlocks, err := w.state.BlocksSince(w.recentBlock)
	if err != nil {
		return err
	}
	for _, id := range removedBlocks {
		diffs, err := w.state.BlockOutputDiffs(id)
		if err != nil {
			return err
		}
		for _, diff := range diffs {
			w.applyDiff(diff, false)
		}
	}
	for _, id := range addedBlocks {
		diffs, err := w.state.BlockOutputDiffs(id)
		if err != nil {
			return err
		}
		for _, diff := range diffs {
			w.applyDiff(diff, true)
		}
	}

	// Get, apply, and store the unconfirmed diffs currently available in the transaction pool.
	w.unconfirmedDiffs = w.tpool.OutputDiffs()
	for _, diff := range w.unconfirmedDiffs {
		w.applyDiff(diff, true)
	}

	w.recentBlock = w.state.CurrentBlock().ID()

	return nil
}
