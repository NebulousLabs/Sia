package wallet

import (
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/sia/components"
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

	mu sync.RWMutex
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

	err = w.Load(filename)
	if err != nil {
		return
	}

	return
}

// Info implements the core.Wallet interface.
func (w *Wallet) Info() (status components.WalletInfo, err error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	status = components.WalletInfo{
		Balance:      w.Balance(false),
		FullBalance:  w.Balance(true),
		NumAddresses: len(w.spendableAddresses),
	}

	return
}
