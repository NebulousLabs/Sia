package wallet

import (
	"errors"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/sia/components"
)

// openTransaction is a type that the wallet uses to track a transaction as it
// adds inputs and other features.
type openTransaction struct {
	transaction *consensus.Transaction
	inputs      []int
}

// spendableOutput keeps track of an output, it's id, and whether or not it's
// been spent yet. Spendable indicates whether the output is available
// according to the blockchain, true if the output is unspent in the blockchain
// and false if the output is spent in the blockchain. The spentCounter
// indicates whether the output has been spent or not. If it's equal to the
// wallet's spent counter, then it has been spent since the previous reset.
type spendableOutput struct {
	spendable    bool
	spentCounter int
	id           consensus.OutputID
	output       consensus.Output
}

// openOutput contains an output and the conditions needed to spend the output,
// including secret keys.
type spendableAddress struct {
	spendableOutputs map[consensus.OutputID]*spendableOutput
	spendConditions  consensus.SpendConditions
	secretKey        crypto.SecretKey
}

// findOutputs returns a set of spendable outputs that add up to at least
// `amount` of coins, returning an error if it cannot. It also returns the
// `total`, which is the sum of all the outputs. It does not adjust the outputs
// in any way.
func (w *Wallet) findOutputs(amount consensus.Currency) (spendableOutputs []*spendableOutput, total consensus.Currency, err error) {
	if amount == consensus.Currency(0) {
		err = errors.New("cannot fund 0 coins") // should this be an error or nil?
		return
	}

	// Iterate through all outputs until enough coins have been assembled.
	for _, spendableAddress := range w.spendableAddresses {
		for _, spendableOutput := range spendableAddress.spendableOutputs {
			if !spendableOutput.spendable || spendableOutput.spentCounter == w.spentCounter {
				continue
			}
			total += spendableOutput.output.Value
			spendableOutputs = append(spendableOutputs, spendableOutput)

			if total >= amount {
				return
			}
		}
	}

	// This code will only be reached if total < amount, meaning insufficient
	// funds.
	//
	// TODO: Figure out some way to return a named error that also contains
	// custom information.
	// err = fmt.Errorf("insufficient funds, requested %v but only have %v", amount, total)
	err = components.LowBalanceErr

	return
}

// Balance implements the core.Wallet interface.
func (w *Wallet) Balance(full bool) (total consensus.Currency) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	// Iterate through all outputs and tally them up.
	for _, spendableAddress := range w.spendableAddresses {
		for _, spendableOutput := range spendableAddress.spendableOutputs {
			if !spendableOutput.spendable {
				continue
			}
			if !full && spendableOutput.spentCounter == w.spentCounter {
				continue
			}
			total += spendableOutput.output.Value
		}
	}
	return
}

// Update implements the core.Wallet interface.
func (w *Wallet) Update(diffs []consensus.OutputDiff) error {
	w.mu.Lock()
	defer w.mu.Unlock()

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

	return nil
}
