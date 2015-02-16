package wallet

import (
	"github.com/NebulousLabs/Sia/consensus"
)

// applyDiff will take the output and either add or delete it from the set of
// outputs known to the wallet. If adding is true, then new outputs will be
// added and expired outputs will be deleted. If adding is false, then new
// outputs will be deleted and expired outputs will be added.
func (w *Wallet) applyDiff(scod consensus.SiacoinOutputDiff, dir consensus.DiffDirection) {
	// See if the output in the diff is known to the wallet.
	key, exists := w.keys[scod.SiacoinOutput.UnlockHash]
	if !exists {
		return
	}

	if scod.Direction == dir {
		// Sanity check - output should not already exist.
		if consensus.DEBUG {
			_, exists := key.outputs[scod.ID]
			if exists {
				panic("adding an output that already exists")
			}
		}

		ko := &knownOutput{
			id:     scod.ID,
			output: scod.SiacoinOutput,
		}
		key.outputs[scod.ID] = ko
	} else {
		if consensus.DEBUG {
			_, exists := key.outputs[scod.ID]
			if !exists {
				panic("trying to delete an output that doesn't exist?")
			}
		}

		delete(key.outputs, scod.ID)
	}
}

// update will synchronize the wallet to the latest set of outputs in the
// consensus set and unconfirmed consensus set. It does this by first removing
// all of the diffs from the previous unconfirmed consensus set, then applying
// all of the diffs between the previous consensus set and current consensus
// set, and then grabbing the current diffs for the unconfirmed consensus set.
// This is only safe if calling tpool.UnconfirmedSiacoinOutputDiffs means that
// the transaction pool will update it's own understanding of the consensus
// set. (This is currently true).
func (w *Wallet) update() error {
	w.state.RLock()
	defer w.state.RUnlock()

	// Remove all of the diffs that have been applied by the unconfirmed set of
	// transactions.
	for _, scod := range w.unconfirmedDiffs {
		w.applyDiff(scod, consensus.DiffRevert)
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
			w.applyDiff(scod, consensus.DiffRevert)
		}
	}
	for _, id := range addedBlocks {
		scods, err := w.state.BlockOutputDiffs(id)
		if err != nil {
			return err
		}
		for _, scod := range scods {
			w.applyDiff(scod, consensus.DiffApply)
		}
	}

	// Get, apply, and store the unconfirmed diffs currently available in the transaction pool.
	w.unconfirmedDiffs = w.tpool.UnconfirmedSiacoinOutputDiffs()
	for _, scod := range w.unconfirmedDiffs {
		w.applyDiff(scod, consensus.DiffApply)
	}

	w.recentBlock = w.state.CurrentBlock().ID()

	return nil
}
