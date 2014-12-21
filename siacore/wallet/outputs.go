package wallet

import (
	"errors"
	"fmt"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/signatures"
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
	output       *consensus.Output
}

// openOutput contains an output and the conditions needed to spend the output,
// including secret keys.
type spendableAddress struct {
	spendableOutputs map[consensus.OutputID]*spendableOutput
	spendConditions  consensus.SpendConditions
	secretKey        signatures.SecretKey
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
	err = fmt.Errorf("insufficient funds, requested %v but only have %v", amount, total)
	return
}

// Balance implements the core.Wallet interface.
func (w *Wallet) Balance(full bool) (total consensus.Currency) {
	w.Lock()
	defer w.Unlock()

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
	w.Lock()
	defer w.Unlock()

	for _, diff := range diffs {
		if diff.New {
			if spendableAddress, exists := w.spendableAddresses[diff.Output.SpendHash]; exists {
				spendableAddress.spendableOutputs[diff.ID] = &spendableOutput{
					spendable: true,
					id:        diff.ID,
					output:    &diff.Output,
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

	return nil
}
