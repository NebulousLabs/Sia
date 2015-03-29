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
				// TODO: RE-add
				// panic("trying to delete an output that doesn't exist?")
			}
		}

		delete(key.outputs, scod.ID)
	}
}

// ReceiveTransactionPoolUpdate gets all of the changes in the confirmed and
// unconfirmed set and uses them to update the balance and transaction history
// of the wallet.
func (w *Wallet) ReceiveTransactionPoolUpdate(revertedBlocks, appliedBlocks []consensus.Block, _ []consensus.Transaction, unconfirmedSiacoinDiffs []consensus.SiacoinOutputDiff) {
	id := w.mu.Lock()
	defer w.mu.Unlock(id)

	for _, diff := range w.unconfirmedDiffs {
		w.applyDiff(diff, consensus.DiffRevert)
	}

	for _, block := range revertedBlocks {
		w.age--

		scods, err := w.state.BlockOutputDiffs(block.ID())
		if err != nil {
			if consensus.DEBUG {
				panic(err)
			}
			continue
		}
		for _, scod := range scods {
			w.applyDiff(scod, consensus.DiffRevert)
		}
	}
	for _, block := range appliedBlocks {
		w.age++

		scods, err := w.state.BlockOutputDiffs(block.ID())
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

	w.unconfirmedDiffs = unconfirmedSiacoinDiffs
	for _, diff := range w.unconfirmedDiffs {
		w.applyDiff(diff, consensus.DiffApply)
	}
}
