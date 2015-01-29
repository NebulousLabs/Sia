package wallet

import (
// "github.com/NebulousLabs/Sia/consensus"
)

func (w *Wallet) update() error {
	/*
		for _, diff := range diffs {
			if diff.New {
				if spendableAddress, exists := w.spendableAddresses[diff.Output.SpendHash]; exists {
					spendableAddress.spendableOutputs[diff.ID] = &spendableOutput{
						spendable: true,
						id:        diff.ID,
						output:    diff.Output,
					}
				}
			} else {
				if spendableAddress, exists := w.spendableAddresses[diff.Output.SpendHash]; exists {
					if spendableOutput, exists := spendableAddress.spendableOutputs[diff.ID]; exists {
						spendableOutput.spendable = false
					} else {
						panic("output should exist!")
					}
				}
			}
		}

		height := w.state.Height()
		if height < w.prevHeight {
			// Since the height is reduced, a bunch of previously unlocked outputs
			// are now locked, so we need to delete them from the spendable outputs
			// list.
			//
			// The for loop iterates over (height, prevHeight]
			for i := height + 1; i <= w.prevHeight; i++ {
				// timelockedSpendableAddresses contains a list of
				// spendableAddresses that get unlocked at the given height. Delete
				// each from the map of spendable addresses.
				for _, spendableAddy := range w.timelockedSpendableAddresses[i] {
					coinAddress := spendableAddy.spendConditions.CoinAddress()
					delete(w.spendableAddresses, coinAddress)
				}
			}
		} else {
			// Since the height is increased, a bunch of previously locked outputs
			// are now unlocked, so we need to add them to the spendable output
			// list.
			//
			// The for loop iterates over (prevHeight, height]
			for i := w.prevHeight + 1; i <= height; i++ {
				for _, spendableAddy := range w.timelockedSpendableAddresses[i] {
					coinAddress := spendableAddy.spendConditions.CoinAddress()
					w.spendableAddresses[coinAddress] = spendableAddy
				}
			}
		}
		w.prevHeight = height
	*/

	return nil
}
