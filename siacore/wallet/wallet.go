package wallet

import (
	"errors"
	// "fmt"
	"strconv"
	"sync"

	"github.com/NebulousLabs/Andromeda/consensus"
	"github.com/NebulousLabs/Andromeda/signatures"
)

// openTransaction is a type that the wallet uses to track a transaction as it
// adds inputs and other features.
type openTransaction struct {
	transaction *consensus.Transaction
	inputs      []uint64
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

// Wallet holds your coins, manages privacy, outputs, ect. The balance reported
// ignores outputs you've already spent even if they haven't made it into the
// blockchain yet.
//
// TODO: Do not ignore refunds until they make it into a block (but later, leave it for now)
type Wallet struct {
	spentCounter       int
	spendableAddresses map[consensus.CoinAddress]*spendableAddress

	transactionCounter int
	transactions       map[string]*openTransaction

	sync.RWMutex
}

// New creates an initializes a Wallet.
func New() (*Wallet, error) {
	return &Wallet{
		spendableAddresses: make(map[consensus.CoinAddress]*spendableAddress),
		transactions:       make(map[string]*openTransaction),
	}, nil
}

// Update implements the core.Wallet interface.
func (w *Wallet) Update(rewound []consensus.Block, applied []consensus.Block) error {
	w.Lock()
	defer w.Unlock()

	// Undo add the changes from blocks that have been rewound.
	for _, b := range rewound {
		for i := len(b.Transactions) - 1; i >= 0; i-- {
			// Mark all outputs that got created (sent to an address in our
			// control) as 'not spendable', because they no longer exist in
			// the blockchain.
			for j, output := range b.Transactions[i].Outputs {
				if spendableAddress, exists := w.spendableAddresses[output.SpendHash]; exists {
					id := b.Transactions[i].OutputID(j)
					if spendableOutput, exists := spendableAddress.spendableOutputs[id]; exists {
						spendableOutput.spendable = false
					} else {
						panic("output should exist")
					}
				}
			}

			// Mark all inputs that we control as 'spendable', because the
			// blockchain is no longer aware that they've been spent.
			for _, input := range b.Transactions[i].Inputs {
				coinAddress := input.SpendConditions.CoinAddress()
				if spendableAddress, exists := w.spendableAddresses[coinAddress]; exists {
					if spendableOutput, exists := spendableAddress.spendableOutputs[input.OutputID]; exists {
						spendableOutput.spendable = true
					} else {
						panic("output should exist")
					}
				}
			}
		}
	}

	// Update spendableOutputs which got spent, and find new outputs which we
	// know how to spend.
	for _, b := range applied {
		for _, t := range b.Transactions {
			// Mark all outputs that got consumed by the block as 'not
			// spendable'
			for _, input := range t.Inputs {
				coinAddress := input.SpendConditions.CoinAddress()
				if spendableAddress, exists := w.spendableAddresses[coinAddress]; exists {
					if spendableOutput, exists := spendableAddress.spendableOutputs[input.OutputID]; exists {
						spendableOutput.spendable = false
					} else {
						panic("output should exist")
					}
				}
			}

			// Mark all outputs that got created (sent to an address in our
			// control) as 'spendable'.
			for j, output := range t.Outputs {
				if spendableAddress, exists := w.spendableAddresses[output.SpendHash]; exists {
					id := t.OutputID(j)
					if spendOutput, exists := spendableAddress.spendableOutputs[id]; exists {
						spendOutput.spendable = true
					} else {
						spendableAddress.spendableOutputs[id] = &spendableOutput{
							spendable: true,
							id:        id,
							output:    &output,
						}
					}
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
	w.spentCounter++
	return nil
}

/*
// Balance implements the core.Wallet interface.
func (w *Wallet) Balance() (consensus.Currency, error) {
	w.RLock()
	defer w.RUnlock()
	return w.balance, nil
}
*/

// CoinAddress implements the core.Wallet interface.
func (w *Wallet) CoinAddress() (coinAddress consensus.CoinAddress, err error) {
	sk, pk, err := signatures.GenerateKeyPair()
	if err != nil {
		return
	}

	newSpendableAddress := &spendableAddress{
		spendableOutputs: make(map[consensus.OutputID]*spendableOutput),
		spendConditions: consensus.SpendConditions{
			NumSignatures: 1,
			PublicKeys:    []signatures.PublicKey{pk},
		},
		secretKey: sk,
	}

	coinAddress = newSpendableAddress.spendConditions.CoinAddress()
	w.spendableAddresses[coinAddress] = newSpendableAddress
	return
}

// RegisterTransaction implements the core.Wallet interface.
func (w *Wallet) RegisterTransaction(t *consensus.Transaction) (id string, err error) {
	w.Lock()
	defer w.Unlock()

	id = strconv.Itoa(w.transactionCounter)
	w.transactionCounter++
	w.transactions[id].transaction = t
	return
}

/*
// FundTransaction implements the core.Wallet interface.
func (w *Wallet) FundTransaction(id string, amount consensus.Currency) error {
	if amount == consensus.Currency(0) {
		return errors.New("cannot fund 0 coins") // should this be an error or nil?
	}
	ot, exists := w.transactions[id]
	if !exists {
		return errors.New("no transaction of given id found")
	}
	t := ot.transaction

	total := consensus.Currency(0)
	var newInputs []consensus.Input
	for id, _ := range w.ownedOutputs {
		// Check if we've already spent the output.
		_, exists := w.spentOutputs[id]
		if exists {
			continue
		}

		// Fetch the output
		output := w.outputs[id].output

		// Create an input for the transaction
		newInput := consensus.Input{
			OutputID:        id,
			SpendConditions: w.spendConditions,
		}
		newInputs = append(newInputs, newInput)

		// See if the value of the inputs has surpassed `amount`.
		total += output.Value
		if total >= amount {
			break
		}
	}

	// Check that enough inputs were added.
	if total < amount {
		return fmt.Errorf("insufficient funds, requested %v but only have %v", amount, total)
	}

	// Add the inputs to the transaction.
	t.Inputs = append(t.Inputs, newInputs...)
	for _, input := range newInputs {
		ot.inputs = append(ot.inputs, uint64(len(t.Inputs)))
		w.spentOutputs[input.OutputID] = struct{}{}
	}

	// Add a refund output if needed.
	if total-amount > 0 {
		t.Outputs = append(
			t.Outputs,
			consensus.Output{
				Value:     total - amount,
				SpendHash: w.spendConditions.CoinAddress(),
			},
		)
	}

	return nil
}
*/

// AddMinerFee implements the core.Wallet interface.
func (w *Wallet) AddMinerFee(id string, fee consensus.Currency) error {
	to, exists := w.transactions[id]
	if !exists {
		return errors.New("no transaction found for given id")
	}

	to.transaction.MinerFees = append(to.transaction.MinerFees, fee)
	return nil
}

// AddOutput implements the core.Wallet interface.
func (w *Wallet) AddOutput(id string, amount consensus.Currency, dest consensus.CoinAddress) error {
	to, exists := w.transactions[id]
	if !exists {
		return errors.New("no transaction found for given id")
	}

	to.transaction.Outputs = append(to.transaction.Outputs, consensus.Output{Value: amount, SpendHash: dest})
	return nil
}
