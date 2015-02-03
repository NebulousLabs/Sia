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
	output consensus.SiacoinOutput

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
	w.update()

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
	w.update()

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
