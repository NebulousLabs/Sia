package wallet

import (
	"strconv"
	"sync"

	"github.com/NebulousLabs/Andromeda/consensus"
	"github.com/NebulousLabs/Andromeda/signatures"
)

// Wallet holds your coins, manages privacy, outputs, ect. The balance reported
// by the wallet does not include coins that you have spent in transactions yet
// haven't been revealed in a block.
//
// TODO: Right now, the Wallet stores all of the outputs itself, because it
// doesn't have access to the state. There should probably be some abstracted
// object which can do that for the Wallet, which is shared between all of the
// things that need to do the lookups. (and type consensus.State would
// implement the interface fulfilling that abstraction)
type Wallet struct {
	secretKey       signatures.SecretKey
	spendConditions consensus.SpendConditions

	balance      consensus.Currency
	ownedOutputs map[consensus.OutputID]struct{}
	spentOutputs map[consensus.OutputID]struct{}
	outputs      map[consensus.OutputID]*consensus.Output

	transactionCounter int
	transactions       map[string]*consensus.Transaction

	sync.RWMutex
}

// New creates an initializes a Wallet.
func New() (w *Wallet, err error) {
	sk, pk, err := signatures.GenerateKeyPair()
	if err != nil {
		return
	}

	w = &Wallet{
		secretKey: sk,
		spendConditions: consensus.SpendConditions{
			NumSignatures: 1,
			PublicKeys:    []signatures.PublicKey{pk},
		},
		ownedOutputs: make(map[consensus.OutputID]struct{}),
		spentOutputs: make(map[consensus.OutputID]struct{}),
		outputs:      make(map[consensus.OutputID]*consensus.Output),
		transactions: make(map[string]*consensus.Transaction),
	}
	return
}

// Update implements the core.Wallet interface.
func (w *Wallet) Update(rewound []consensus.Block, applied []consensus.Block) error {
	w.Lock()
	defer w.Unlock()

	ca := w.spendConditions.CoinAddress()

	// Remove all of the owned outputs created in the rewound blocks. Do not
	// change the spent outputs map.
	for _, b := range rewound {
		for i := len(b.Transactions) - 1; i >= 0; i-- {
			// Remove all outputs that got created by this block.
			for j, _ := range b.Transactions[i].Outputs {
				id := b.Transactions[i].OutputID(j)
				delete(w.ownedOutputs, id)
			}

			// Re-add all inputs that got consumed by this block.
			for _, input := range b.Transactions[i].Inputs {
				if ca == input.SpendConditions.CoinAddress() {
					w.balance += w.outputs[input.OutputID].Value
					w.ownedOutputs[input.OutputID] = struct{}{}
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
				delete(w.ownedOutputs, input.OutputID)
			}

			// Add all of the outputs that got created by this block.
			for i, output := range t.Outputs {
				if ca == output.SpendHash {
					id := t.OutputID(i)
					w.ownedOutputs[id] = struct{}{}
					w.outputs[id] = &output
					w.balance += output.Value
				}
			}
		}
	}

	return nil
}

// Reset implements the core.Wallet interface.
func (w *Wallet) Reset() error {
	w.Lock()
	defer w.Unlock()

	for id := range w.spentOutputs {
		// Add the spent output back into the balance if it's currently an
		// owned output.
		if _, exists := w.ownedOutputs[id]; exists {
			w.balance += w.outputs[id].Value
		}
		delete(w.spentOutputs, id)
	}
	return nil
}

// Balance implements the core.Wallet interface.
func (w *Wallet) Balance() (consensus.Currency, error) {
	w.RLock()
	defer w.RUnlock()
	return w.balance, nil
}

// NewTransaction implements the core.Wallet interface.
func (w *Wallet) NewTransaction() (id string, err error) {
	w.Lock()
	defer w.Unlock()

	id = strconv.Itoa(w.transactionCounter)
	w.transactionCounter++
	w.transactions[id] = new(consensus.Transaction)
	return
}

func (w *Wallet) RegisterTransaction(t *consensus.Transaction) (id string, err error) {
	w.Lock()
	defer w.Unlock()

	id = strconv.Itoa(w.transactionCounter)
	w.transactionCounter++
	w.transactions[id] = t
	return
}
