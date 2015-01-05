package wallet

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/signatures"
)

// AddressKey is how we serialize and store spendable addresses on
// disk.
type AddressKey struct {
	SpendConditions consensus.SpendConditions
	SecretKey       signatures.SecretKey
}

// Wallet holds your coins, manages privacy, outputs, ect. The balance reported
// ignores outputs you've already spent even if they haven't made it into the
// blockchain yet.
//
// The spentCounter is used to indicate which transactions have been spent but
// have not appeared in the blockchain. It's used as an int for an easy reset.
// Each transaction also has a spent counter. If the transaction's spent
// counter is equal to the wallet's spent counter, then the transaction has
// been spent since the last reset. Upon reset, the wallet's spent counter is
// incremented, which means all transactions will no longer register as having
// been spent since the last reset.
//
// Wallet.transactions is a list of transactions that are currently being built
// within the wallet. The transactionCounter ensures that each
// transaction-in-progress gets a unique ID.
type Wallet struct {
	saveFilename string

	spentCounter       int
	spendableAddresses map[consensus.CoinAddress]*spendableAddress

	transactionCounter int
	transactions       map[string]*openTransaction

	sync.RWMutex
}

type Status struct {
	Balance      consensus.Currency
	FullBalance  consensus.Currency
	NumAddresses int
}

// New creates a new wallet, loading any known addresses from the input file
// name and then using the file to save in the future.
func New(filename string) (w *Wallet, err error) {
	w = &Wallet{
		spentCounter:       1,
		saveFilename:       filename,
		spendableAddresses: make(map[consensus.CoinAddress]*spendableAddress),
		transactions:       make(map[string]*openTransaction),
	}

	// Check if the file exists, then read it into memory.
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

// Reset implements the core.Wallet interface.
func (w *Wallet) Reset() error {
	w.Lock()
	defer w.Unlock()
	w.spentCounter++
	return nil
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

// Info implements the core.Wallet interface.
func (w *Wallet) Info() ([]byte, error) {
	w.RLock()
	defer w.RUnlock()

	status := Status{
		Balance:     w.Balance(false),
		FullBalance: w.Balance(true),
	}
	status.NumAddresses = len(w.spendableAddresses)

	return json.Marshal(status)
}
