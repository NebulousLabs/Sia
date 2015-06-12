package wallet

import (
	"errors"
	"fmt"
	"os"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// AgeDelay indicates how long the wallet will wait before allowing the
	// user to double-spend a transaction under standard circumstances. The
	// rationale is that most transactions are meant to be submitted to the
	// blockchain immediately, and ones that take more than AgeDelay blocks
	// have probably failed in some way.
	AgeDelay = 80

	// TransactionFee is yet another deprecated-on-arrival constant that says
	// how large the transaction fees should be. This should really be a
	// function supplied by the transaction pool.
	TransactionFee = 10
)

// A Wallet uses the state and transaction pool to track the unconfirmed
// balance of a user. All of the keys are stored in 'saveDir'/wallet.dat.
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
	state            modules.ConsensusSet
	tpool            modules.TransactionPool
	unconfirmedDiffs []modules.SiacoinOutputDiff

	// Location of the wallet directory, for saving and loading keys.
	saveDir string

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
	//
	// Visible keys will be displayed to the user.
	consensusHeight  types.BlockHeight
	age              int
	keys             map[types.UnlockHash]*key
	timelockedKeys   map[types.BlockHeight][]types.UnlockHash
	visibleAddresses map[types.UnlockHash]struct{}
	siafundAddresses map[types.UnlockHash]struct{}
	siafundOutputs   map[types.SiafundOutputID]types.SiafundOutput

	// transactions is a list of transactions that are currently being built by
	// the wallet. Each transaction has a unique id, which is enforced by the
	// transactionCounter.
	transactionCounter int
	transactions       map[string]*openTransaction

	subscribers []chan struct{}

	mu *sync.RWMutex
}

// New creates a new wallet, loading any known addresses from the input file
// name and then using the file to save in the future.
func New(cs modules.ConsensusSet, tpool modules.TransactionPool, saveDir string) (w *Wallet, err error) {
	if cs == nil {
		err = errors.New("wallet cannot use a nil state")
		return
	}
	if tpool == nil {
		err = errors.New("wallet cannot use a nil transaction pool")
		return
	}

	w = &Wallet{
		state: cs,
		tpool: tpool,

		saveDir: saveDir,

		age:              AgeDelay + 100,
		keys:             make(map[types.UnlockHash]*key),
		timelockedKeys:   make(map[types.BlockHeight][]types.UnlockHash),
		visibleAddresses: make(map[types.UnlockHash]struct{}),

		transactions: make(map[string]*openTransaction),

		mu: sync.New(modules.SafeMutexDelay, 1),
	}

	// Create the wallet folder.
	err = os.MkdirAll(saveDir, 0700)
	if err != nil {
		return
	}

	// Try to load a previously saved wallet file. If it doesn't exist, assume
	// that we're creating a new wallet file.
	// TODO: log warning if no file found?
	err = w.load()
	if os.IsNotExist(err) {
		err = nil
		// No wallet file exists... make a visible address for the user.
		_, _, err = w.coinAddress(true)
		if err != nil {
			return nil, err
		}
	}
	if err != nil {
		err = fmt.Errorf("couldn't load wallet file %s: %v", saveDir, err)
		// TODO: try to recover from wallet.backup?
		return
	}

	w.tpool.TransactionPoolSubscribe(w)

	return
}

func (w *Wallet) Close() error {
	id := w.mu.RLock()
	defer w.mu.RUnlock(id)
	return w.save()
}

// SpendCoins creates a transaction sending 'amount' to 'dest'. The transaction
// is submitted to the transaction pool and is also returned.
func (w *Wallet) SpendCoins(amount types.Currency, dest types.UnlockHash) (t types.Transaction, err error) {
	// Create and send the transaction.
	output := types.SiacoinOutput{
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
	_, _, err = w.AddSiacoinOutput(id, output)
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
