package wallet

import (
	"errors"
	"fmt"
	"time"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/sync"
)

const (
	// AgeDelay indicates how long the wallet will wait before allowing the
	// user to double-spend a transaction under standard circumstances. The
	// rationale is that most transactions are meant to be submitted to the
	// blockchain immediately, and ones that take more than AgeDelay blocks
	// have probably failed in some way.
	AgeDelay = 80
)

// A Wallet uses the state and transaction pool to track the unconfirmed
// balance of a user. All of the keys are stored in the file 'filename'.
//
// One feature of the wallet is preventing accidental double spends. The wallet
// will block an output from being spent if it has been spent in the last
// 'AgeDelay' blocks. This is managed by tracking a global age for the wallet
// and then an age for each output, set to the age of the wallet that the
// output was most recently spent. If the wallet is 'AgeDelay' blocks older
// than an output, then the output can be spent again.
//
// A second feature of the wallet is the transaction builder, which is a series
// of functions that can be used to build independent transactions for use with
// untrusted parties. The transactions can be cobbled together piece by piece
// and then signed. When using the transaction builder, the wallet will always
// have exact outputs (by creating another transaction first if needed) and
// thus the transaction does not need to be spent for the transaction builder
// to be able to use any refunds.
type Wallet struct {
	state            *consensus.State
	tpool            modules.TransactionPool
	recentBlock      consensus.BlockID
	unconfirmedDiffs []consensus.SiacoinOutputDiff

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
	keys           map[consensus.UnlockHash]*key
	timelockedKeys map[consensus.BlockHeight][]consensus.UnlockHash

	// transactions is a list of transactions that are currently being built by
	// the wallet. Each transaction has a unique id, which is enforced by the
	// transactionCounter.
	transactionCounter int
	transactions       map[string]*openTransaction

	mu sync.RWMutex
}

// New creates a new wallet, loading any known addresses from the input file
// name and then using the file to save in the future.
func New(state *consensus.State, tpool modules.TransactionPool, filename string) (w *Wallet, err error) {
	if state == nil {
		err = errors.New("wallet cannot use a nil state")
		return
	}
	if tpool == nil {
		err = errors.New("wallet cannot use a nil transaction pool")
		return
	}

	// Get the genesis block to set as 'recent block'.
	genesisBlock, exists := state.BlockAtHeight(0)
	if !exists {
		err = errors.New("could not fetch genesis block")
		return
	}

	w = &Wallet{
		state:       state,
		tpool:       tpool,
		recentBlock: genesisBlock.ID(),

		filename: filename,

		age:            AgeDelay + 100,
		keys:           make(map[consensus.UnlockHash]*key),
		timelockedKeys: make(map[consensus.BlockHeight][]consensus.UnlockHash),

		transactions: make(map[string]*openTransaction),

		mu: sync.New(5*time.Second, 0),
	}

	// If the wallet file already exists, try to load it.
	// TODO: log warning if no file found?
	if fileExists(filename) {
		// lock not necessary here because no one else has access to w
		err = w.load(filename)
		if err != nil {
			err = fmt.Errorf("couldn't load wallet file %s: %v", filename, err)
			return
		}
	}

	return
}

// SpendCoins creates a transaction sending 'amount' to 'dest'. The transaction
// is submitted to the miner pool, but is also returned.
//
// TODO: Since the style of FundTransaction has changed to work with untrusted
// parties, SpendCoins has actually become inefficient, creating 2 transactions
// and extra outputs where something slimmer would do the same job just as
// well.
func (w *Wallet) SpendCoins(amount consensus.Currency, dest consensus.UnlockHash) (t consensus.Transaction, err error) {
	// Create and send the transaction.
	output := consensus.SiacoinOutput{
		Value:      amount,
		UnlockHash: dest,
	}
	id, err := w.RegisterTransaction(t)
	if err != nil {
		return
	}
	_, err = w.FundTransaction(id, amount)
	if err != nil {
		return
	}
	_, _, err = w.AddOutput(id, output)
	if err != nil {
		return
	}
	t, err = w.SignTransaction(id, true)
	if err != nil {
		return
	}
	err = w.tpool.AcceptTransaction(t)
	if err != nil {
		return
	}

	return
}
