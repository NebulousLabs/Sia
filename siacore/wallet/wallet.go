package wallet

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
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

// Wallet holds your coins, manages privacy, outputs, ect. The balance reported
// ignores outputs you've already spent even if they haven't made it into the
// blockchain yet.
type Wallet struct {
	saveFilename string

	spentCounter       int
	spendableAddresses map[consensus.CoinAddress]*spendableAddress

	transactionCounter int
	transactions       map[string]*openTransaction

	sync.Mutex
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
LoopBreak:
	for _, spendableAddress := range w.spendableAddresses {
		for _, spendableOutput := range spendableAddress.spendableOutputs {
			if !spendableOutput.spendable || spendableOutput.spentCounter == w.spentCounter {
				continue
			}
			total += spendableOutput.output.Value
			spendableOutputs = append(spendableOutputs, spendableOutput)

			// Break once
			if total >= amount {
				break LoopBreak
			}
		}
	}

	// Check that enough inputs were added.
	if total < amount {
		err = fmt.Errorf("insufficient funds, requested %v but only have %v", amount, total)
		return
	}

	return
}

// New creates a new wallet, loading any known addresses from the input file
// name and then using the file to save in the future.
func New(filename string) (w *Wallet, err error) {
	w = &Wallet{
		saveFilename:       filename,
		spendableAddresses: make(map[consensus.CoinAddress]*spendableAddress),
		transactions:       make(map[string]*openTransaction),
	}

	// Check if the file exists.
	if _, err = os.Stat(filename); os.IsNotExist(err) {
		err = nil
		return
	}

	contents, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}

	// Unmarshal the spendable addresses and put them into the wallet.
	var keys []AddressKey
	err = encoding.Unmarshal(contents, &keys)
	if err != nil {
		return
	}
	for _, key := range keys {
		newSpendableAddress := &spendableAddress{
			spendableOutputs: make(map[consensus.OutputID]*spendableOutput),
			spendConditions:  key.SpendConditions,
			secretKey:        key.SecretKey,
		}
		w.spendableAddresses[key.SpendConditions.CoinAddress()] = newSpendableAddress
	}
	return
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
					spendableAddress.spendableOutputs[id] = &spendableOutput{
						spendable: true,
						id:        id,
						output:    &output,
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

// Balance implements the core.Wallet interface.
func (w *Wallet) Balance(full bool) (total consensus.Currency) {
	w.Lock()
	defer w.Unlock()

	// Iterate through all outputs and tally them up.
	for _, spendableAddress := range w.spendableAddresses {
		for _, spendableOutput := range spendableAddress.spendableOutputs {
			if !full && (!spendableOutput.spendable || spendableOutput.spentCounter == w.spentCounter) {
				continue
			}
			total += spendableOutput.output.Value
		}
	}
	return
}

// timelockedCoinAddress returns a CoinAddress with a timelock, as well as the
// conditions needed to spend it.
func (w *Wallet) timelockedCoinAddress(release consensus.BlockHeight) (spendConditions consensus.SpendConditions, err error) {
	sk, pk, err := signatures.GenerateKeyPair()
	if err != nil {
		return
	}

	spendConditions = consensus.SpendConditions{
		TimeLock:      release,
		NumSignatures: 1,
		PublicKeys:    []signatures.PublicKey{pk},
	}

	newSpendableAddress := &spendableAddress{
		spendableOutputs: make(map[consensus.OutputID]*spendableOutput),
		spendConditions:  spendConditions,
		secretKey:        sk,
	}

	coinAddress := spendConditions.CoinAddress()
	w.spendableAddresses[coinAddress] = newSpendableAddress
	return
}

// CoinAddress implements the core.Wallet interface.
func (w *Wallet) CoinAddress() (coinAddress consensus.CoinAddress, err error) {
	w.Lock()
	defer w.Unlock()

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
	err = w.Save()
	return
}

// RegisterTransaction implements the core.Wallet interface.
func (w *Wallet) RegisterTransaction(t consensus.Transaction) (id string, err error) {
	w.Lock()
	defer w.Unlock()

	id = strconv.Itoa(w.transactionCounter)
	w.transactionCounter++
	w.transactions[id].transaction = &t
	return
}

// FundTransaction implements the core.Wallet interface.
func (w *Wallet) FundTransaction(id string, amount consensus.Currency) error {
	w.Lock()
	defer w.Unlock()

	// Get the transaction.
	ot, exists := w.transactions[id]
	if !exists {
		return errors.New("no transaction of given id found")
	}
	t := ot.transaction

	// Get the set of outputs.
	spendableOutputs, total, err := w.findOutputs(amount)
	if err != nil {
		return err
	}

	// Create and add all of the inputs.
	for _, spendableOutput := range spendableOutputs {
		spendableAddress := w.spendableAddresses[spendableOutput.output.SpendHash]
		newInput := consensus.Input{
			OutputID:        spendableOutput.id,
			SpendConditions: spendableAddress.spendConditions,
		}
		ot.inputs = append(ot.inputs, len(t.Inputs))
		t.Inputs = append(t.Inputs, newInput)
	}

	// Add a refund output if needed.
	if total-amount > 0 {
		coinAddress, err := w.CoinAddress()
		if err != nil {
			return err
		}
		t.Outputs = append(
			t.Outputs,
			consensus.Output{
				Value:     total - amount,
				SpendHash: coinAddress,
			},
		)
	}
	return nil
}

// AddMinerFee implements the core.Wallet interface.
func (w *Wallet) AddMinerFee(id string, fee consensus.Currency) error {
	w.Lock()
	defer w.Unlock()

	to, exists := w.transactions[id]
	if !exists {
		return errors.New("no transaction found for given id")
	}

	to.transaction.MinerFees = append(to.transaction.MinerFees, fee)
	return nil
}

// AddOutput implements the core.Wallet interface.
func (w *Wallet) AddOutput(id string, o consensus.Output) error {
	w.Lock()
	defer w.Unlock()

	to, exists := w.transactions[id]
	if !exists {
		return errors.New("no transaction found for given id")
	}

	to.transaction.Outputs = append(to.transaction.Outputs, o)
	return nil
}

// AddTimelockedRefund implements the core.Wallet interface.
func (w *Wallet) AddTimelockedRefund(id string, amount consensus.Currency, release consensus.BlockHeight) (spendConditions consensus.SpendConditions, refundIndex uint64, err error) {
	w.Lock()
	defer w.Unlock()

	// Get the transaction
	ot, exists := w.transactions[id]
	if !exists {
		err = errors.New("no transaction found for given id")
		return
	}
	t := ot.transaction

	// Get a frozen coin address.
	spendConditions, err = w.timelockedCoinAddress(release)
	if err != nil {
		return
	}

	// Add the output to the transaction
	output := consensus.Output{
		Value:     amount,
		SpendHash: spendConditions.CoinAddress(),
	}
	refundIndex = uint64(len(t.Outputs))
	t.Outputs = append(t.Outputs, output)
	return
}

// AddFileContract implements the core.Wallet interface.
func (w *Wallet) AddFileContract(id string, fc consensus.FileContract) error {
	w.Lock()
	defer w.Unlock()

	to, exists := w.transactions[id]
	if !exists {
		return errors.New("no transaction found for given id")
	}

	to.transaction.FileContracts = append(to.transaction.FileContracts, fc)
	return nil
}

// AddStorageProof implements the core.Wallet interface.
func (w *Wallet) AddStorageProof(id string, sp consensus.StorageProof) error {
	w.Lock()
	defer w.Unlock()

	to, exists := w.transactions[id]
	if !exists {
		return errors.New("no transaction found for given id")
	}

	to.transaction.StorageProofs = append(to.transaction.StorageProofs, sp)
	return nil
}

// AddArbitraryData implements the core.Wallet interface.
func (w *Wallet) AddArbitraryData(id string, arb string) error {
	w.Lock()
	defer w.Unlock()

	to, exists := w.transactions[id]
	if !exists {
		return errors.New("no transaction found for given id")
	}

	to.transaction.ArbitraryData = append(to.transaction.ArbitraryData, arb)
	return nil
}

// SignTransaction implements the core.Wallet interface.
func (w *Wallet) SignTransaction(id string, wholeTransaction bool) (transaction consensus.Transaction, err error) {
	w.Lock()
	defer w.Unlock()

	// Fetch the transaction.
	ot, exists := w.transactions[id]
	if !exists {
		err = errors.New("no transaction found for given id")
		return
	}
	transaction = *ot.transaction

	// Get the coveredfields struct.
	var coveredFields consensus.CoveredFields
	if wholeTransaction {
		coveredFields = consensus.CoveredFields{WholeTransaction: true}
	} else {
		for i := range transaction.MinerFees {
			coveredFields.MinerFees = append(coveredFields.MinerFees, uint64(i))
		}
		for i := range transaction.Inputs {
			coveredFields.Inputs = append(coveredFields.Inputs, uint64(i))
		}
		for i := range transaction.Outputs {
			coveredFields.Outputs = append(coveredFields.Outputs, uint64(i))
		}
		for i := range transaction.FileContracts {
			coveredFields.Contracts = append(coveredFields.Contracts, uint64(i))
		}
		for i := range transaction.StorageProofs {
			coveredFields.StorageProofs = append(coveredFields.StorageProofs, uint64(i))
		}
		for i := range transaction.ArbitraryData {
			coveredFields.ArbitraryData = append(coveredFields.ArbitraryData, uint64(i))
		}

		// TODO: Should we also sign all of the known signatures?
	}

	// For each input in the transaction that we added, provide a signature.
	for _, inputIndex := range ot.inputs {
		input := transaction.Inputs[inputIndex]
		sig := consensus.TransactionSignature{
			InputID:        input.OutputID,
			CoveredFields:  coveredFields,
			PublicKeyIndex: 0,
		}
		transaction.Signatures = append(transaction.Signatures, sig)

		// Hash the transaction according to the covered fields and produce the
		// cryptographic signature.
		secKey := w.spendableAddresses[input.SpendConditions.CoinAddress()].secretKey
		sigHash := transaction.SigHash(len(transaction.Signatures) - 1)
		transaction.Signatures[len(transaction.Signatures)-1].Signature, err = signatures.SignBytes(sigHash[:], secKey)
	}

	// Delete the open transaction.
	delete(w.transactions, id)

	return
}

// AddressKey is how we serialize and store spendable addresses on
// disk.
type AddressKey struct {
	SpendConditions consensus.SpendConditions
	SecretKey       signatures.SecretKey
}

// Save implements the core.Wallet interface.
func (w *Wallet) Save() (err error) {
	// Add every known spendable address + secret key.
	var i int
	keys := make([]AddressKey, len(w.spendableAddresses))
	for _, spendableAddress := range w.spendableAddresses {
		key := AddressKey{
			SpendConditions: spendableAddress.spendConditions,
			SecretKey:       spendableAddress.secretKey,
		}
		keys[i] = key
		i++
	}

	//  write the file
	fileData := encoding.Marshal(keys)
	if err != nil {
		return
	}
	err = ioutil.WriteFile(w.saveFilename, fileData, 0666)
	return
}
