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

	w.Load(filename)

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
