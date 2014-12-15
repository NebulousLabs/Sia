package wallet

import (
	"github.com/NebulousLabs/Andromeda/consensus"
	"github.com/NebulousLabs/Andromeda/signatures"
)

// Wallet is the struct used in the wallet package. Though it seems like
// stuttering, most users are going to be calling functions like `w :=
// wallet.New()`
//
// TODO: Right now, the Wallet stores all of the outputs itself, because it
// doesn't have access to the state. There should probably be some abstracted
// object which can do that for the Wallet, which is shared between all of the
// things that need to do the lookups. (and type consensus.State would
// implement the interface fulfilling that abstraction)
type Wallet struct {
	SecretKey       signatures.SecretKey
	SpendConditions consensus.SpendConditions

	Balance      consensus.Currency
	OwnedOutputs map[consensus.OutputID]struct{}
	SpentOutputs map[consensus.OutputID]struct{}
	OutputMap    map[consensus.OutputID]*consensus.Output
}

// New creates an initializes a Wallet.
func New() (w *Wallet, err error) {
	sk, pk, err := signatures.GenerateKeyPair()
	if err != nil {
		return
	}

	w = &Wallet{
		SecretKey: sk,
		SpendConditions: consensus.SpendConditions{
			NumSignatures: 1,
			PublicKeys:    []signatures.PublicKey{pk},
		},
		OwnedOutputs: make(map[consensus.OutputID]struct{}),
		SpentOutputs: make(map[consensus.OutputID]struct{}),
		OutputMap:    make(map[consensus.OutputID]*consensus.Output),
	}
	return
}

// Update implements the core.Wallet interface.
func (w *Wallet) Update(rewound []consensus.Block, applied []consensus.Block) error {
	ca := w.SpendConditions.CoinAddress()

	// Remove all of the owned outputs created in the rewound blocks. Do not
	// change the spent outputs map.
	for _, b := range rewound {
		for i := len(b.Transactions) - 1; i >= 0; i-- {
			// Remove all outputs that got created by this block.
			for j, _ := range b.Transactions[i].Outputs {
				id := b.Transactions[i].OutputID(j)
				delete(w.OwnedOutputs, id)
			}

			// Re-add all inputs that got consumed by this block.
			for _, input := range b.Transactions[i].Inputs {
				if ca == input.SpendConditions.CoinAddress() {
					w.OwnedOutputs[input.OutputID] = struct{}{}
				}
			}
		}
	}

	// Add all of the owned outputs created in applied blocks, and remove all
	// of the owned outputs that got consumed.
	for _, b := range applied {
		for _, t := range b.Transactions {
			// Remove all the outputs that got consumed by this block.
			for _, input := range t.Inputs {
				delete(w.OwnedOutputs, input.OutputID)
			}

			// Add all of the outputs that got created by this block.
			for i, output := range t.Outputs {
				if ca == output.SpendHash {
					id := t.OutputID(i)
					w.OwnedOutputs[id] = struct{}{}
					w.OutputMap[id] = &output
				}
			}
		}
	}

	return nil
}
