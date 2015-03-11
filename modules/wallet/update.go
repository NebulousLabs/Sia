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
		// If the output is already known, ignore it.
		_, exists := key.outputs[scod.ID]
		if exists {
			return
		}

		// Add the output.
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
	// Because we were running into problems, the amount of time that the state
	// lock is held has been minimized. Grab all of the necessary diffs under a
	// lock before doing any computation.
	counter := w.state.RLock("wallet update")
	newUnconfirmedDiffs := w.tpool.UnconfirmedSiacoinOutputDiffs()
	removedBlocks, addedBlocks, err := w.state.BlocksSince(w.recentBlock)
	w.state.RUnlock("wallet update", counter)
	if err != nil {
		return err
	}

	// Remove all of the diffs that have been applied by the unconfirmed set of
	// transactions.
	for i := len(w.unconfirmedDiffs) - 1; i >= 0; i-- {
		w.applyDiff(w.unconfirmedDiffs[i], consensus.DiffRevert)
	}

	// Apply the diffs in the consensus set that have happened since the last
	// update.
	for _, id := range removedBlocks {
		w.age--

		scods, err := w.state.BlockOutputDiffs(id)
		if err != nil {
			return err
		}
		for _, scod := range scods {
			w.applyDiff(scod, consensus.DiffRevert)
		}
	}
	for _, id := range addedBlocks {
		w.age++

		scods, err := w.state.BlockOutputDiffs(id)
		if err != nil {
			if consensus.DEBUG {
				panic(err)
			}
			continue
		}
		for _, scod := range scods {
			w.applyDiff(scod, consensus.DiffApply)
		}
	}

	// Get, apply, and store the unconfirmed diffs currently available in the
	// transaction pool.
	w.unconfirmedDiffs = newUnconfirmedDiffs
	for _, scod := range w.unconfirmedDiffs {
		w.applyDiff(scod, consensus.DiffApply)
	}

	w.recentBlock = w.state.CurrentBlock().ID()

	return nil
}
