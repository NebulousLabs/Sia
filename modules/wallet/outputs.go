package wallet

import (
	"errors"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

// spendableOutput keeps track of an output, it's id, and whether or not it's
// been spent yet. Spendable indicates whether the output is available
// according to the blockchain, true if the output is unspent in the blockchain
// and false if the output is spent in the blockchain. The spentCounter
// indicates whether the output has been spent or not. If it's equal to the
// wallet's spent counter, then it has been spent since the previous reset.
type knownOutput struct {
	id     consensus.OutputID
	output consensus.Output

	spendable bool
	age       int
}

// openOutput contains an output and the conditions needed to spend the output,
// including secret keys.
type key struct {
	spendable       bool
	spendConditions consensus.SpendConditions
	secretKey       crypto.SecretKey

	outputs map[consensus.OutputID]*knownOutput
}

// findOutputs returns a set of spendable outputs that add up to at least
// `amount` of coins, returning an error if it cannot. It also returns the
// `total`, which is the sum of all the outputs. It does not adjust the outputs
// in any way.
func (w *Wallet) findOutputs(amount consensus.Currency) (knownOutputs []*knownOutput, total consensus.Currency, err error) {
	if amount == consensus.Currency(0) {
		err = errors.New("cannot fund 0 coins") // should this be an error or nil?
		return
	}

	// Iterate through all outputs until enough coins have been assembled.
	for _, key := range w.keys {
		if !key.spendable {
			continue
		}
		for _, knownOutput := range key.outputs {
			if !knownOutput.spendable || knownOutput.age > w.age-AgeDelay {
				continue
			}
			total += knownOutput.output.Value
			knownOutputs = append(knownOutputs, knownOutput)

			if total >= amount {
				return
			}
		}
	}

	// This code will only be reached if total < amount, meaning insufficient
	// funds.
	err = modules.LowBalanceErr
	return
}

// Balance returns the number of coins available to the wallet. If `full` is
// set to false, only coins that can be spent immediately are counted.
// Otherwise, all coins that could be spent are counted (including those that
// have already been spent but the transactions haven't been added to the
// transaction pool or blockchain)
func (w *Wallet) Balance(full bool) (total consensus.Currency) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	// Iterate through all outputs and tally them up.
	for _, key := range w.keys {
		if !key.spendable && !full {
			continue
		}
		for _, knownOutput := range key.outputs {
			if !knownOutput.spendable {
				continue
			}
			if !full && knownOutput.age > w.age-AgeDelay {
				continue
			}
			total += knownOutput.output.Value
		}
	}
	return
}

// Update implements the core.Wallet interface.
func (w *Wallet) Update(diffs []consensus.OutputDiff) error {
	w.mu.Lock()
	defer w.mu.Unlock()

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
