package wallet

import (
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
)

const (
	// AgeDelay indicates how long the wallet will wait before allowing the
	// user to double-spend a transaction under standard circumstances. The
	// rationale is that most transactions are meant to be submitted to the
	// blockchain immediately, and ones that take more than AgeDelay blocks
	// have probably failed in some way. There are means to increase the
	// AgeDelay for specific transactions.
	AgeDelay = 80
)

// The wallet contains a list of addresses and the methods to spend them (the
// keys), as well as an interactive way to construct and sign transactions.
type Wallet struct {
	state *consensus.State

	// Location of the wallet's file, for saving and loading keys.
	filename string

	// A key contains all the information necessary to spend a particular
	// address, as well as all the known outputs that use the address.
	//
	// age is a tool to determine whether or not an output can be spent. When
	// an output is spent by the wallet, the age of the output is marked equal
	// to the age of the wallet. It will not be spent again until the age is
	// `AgeDelay` less than the wallet. The wallet ages by 1 every block. The
	// wallet can also be manually aged, which is a convenient and efficient
	// way of resetting spent outputs. Transactions are not intended to be
	// broadcast for a while can be given an age that is much greater than the
	// wallet.
	//
	// Timelocked keys is a list of addresses found in `keys` that can't be
	// spent until a certain height. The wallet will use `timelockedKeys` to
	// mark keys as unspendable until the timelock has lifted.
	age            int
	keys           map[consensus.CoinAddress]*key
	timelockedKeys map[consensus.BlockHeight][]consensus.CoinAddress

	// transactions is a list of transactions that are currently being built by
	// the wallet. Each transaction has a unique id, which is enforced by the
	// transactionCounter.
	transactionCounter int
	transactions       map[string]*openTransaction

	mu sync.RWMutex
}

// New creates a new wallet, loading any known addresses from the input file
// name and then using the file to save in the future.
func New(state *consensus.State, filename string) (w *Wallet, err error) {
	w = &Wallet{
		state: state,

		filename: filename,

		age:            1,
		keys:           make(map[consensus.CoinAddress]key),
		timelockedKeys: make(map[consensus.BlockHeight][]consensus.CoinAddress),

		transactions: make(map[string]*openTransaction),
	}

	err = w.Load(filename)
	if err != nil {
		return
	}

	return
}

// SpendCoins creates a transaction sending 'amount' to 'dest'. The transaction
// is submitted to the miner pool, but is also returned.
func (w *Wallet) SpendCoins(amount consensus.Currency, dest consensus.CoinAddress) (t consensus.Transaction, err error) {
	// Create and send the transaction.
	output := consensus.Output{
		Value:     amount,
		SpendHash: dest,
	}
	id, err := w.RegisterTransaction(t)
	if err != nil {
		return
	}
	err = w.FundTransaction(id, amount+minerFee)
	if err != nil {
		return
	}
	err = w.AddOutput(id, output)
	if err != nil {
		return
	}
	t, err = w.SignTransaction(id, true)
	if err != nil {
		return
	}
	err = w.state.AcceptTransaction(t)
	if err != nil {
		return
	}

	return
}
