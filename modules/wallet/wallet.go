package wallet

import (
	"errors"
	"fmt"
	"log"
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

type Wallet struct {
	unlocked bool
	settings WalletSettings

	state            modules.ConsensusSet
	tpool            modules.TransactionPool
	unconfirmedDiffs []modules.SiacoinOutputDiff
	siafundPool      types.Currency

	consensusHeight  types.BlockHeight
	age              int
	keys             map[types.UnlockHash]*key
	timelockedKeys   map[types.BlockHeight][]types.UnlockHash
	visibleAddresses map[types.UnlockHash]struct{}
	siafundAddresses map[types.UnlockHash]struct{}
	siafundOutputs   map[types.SiafundOutputID]types.SiafundOutput

	trackedKeys map[types.UnlockHash]struct{}

	persistDir string
	log        *log.Logger
	mu         *sync.RWMutex
}

// New creates a new wallet, loading any known addresses from the input file
// name and then using the file to save in the future.
func New(cs modules.ConsensusSet, tpool modules.TransactionPool, persistDir string) (*Wallet, error) {
	// Check for nil dependencies.
	if cs == nil {
		return nil, errors.New("wallet cannot use a nil state")
	}
	if tpool == nil {
		return nil, errors.New("wallet cannot use a nil transaction pool")
	}

	// Initialize the data structure.
	w := &Wallet{
		state: cs,
		tpool: tpool,

		age:              AgeDelay * 2,
		keys:             make(map[types.UnlockHash]*key),
		timelockedKeys:   make(map[types.BlockHeight][]types.UnlockHash),
		visibleAddresses: make(map[types.UnlockHash]struct{}),
		siafundAddresses: make(map[types.UnlockHash]struct{}),
		siafundOutputs:   make(map[types.SiafundOutputID]types.SiafundOutput),

		persistDir: persistDir,
		mu:         sync.New(modules.SafeMutexDelay, 1),
	}

	// Initialize the persistent structures.
	err := w.initPersist()
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("couldn't load wallet file %s: %v", persistDir, err)
	}

	w.tpool.TransactionPoolSubscribe(w)

	return w, nil
}

// Close will save the wallet before shutting down.
func (w *Wallet) Close() error {
	id := w.mu.RLock()
	defer w.mu.RUnlock(id)
	return w.save()
}

// SendCoins creates a transaction sending 'amount' to 'dest'. The transaction
// is submitted to the transaction pool and is also returned.
func (w *Wallet) SendCoins(amount types.Currency, dest types.UnlockHash) ([]types.Transaction, error) {
	tpoolFee := types.NewCurrency64(10).Mul(types.SiacoinPrecision)
	output := types.SiacoinOutput{
		Value:      amount,
		UnlockHash: dest,
	}

	txnBuilder := w.StartTransaction()
	err := txnBuilder.FundSiacoins(amount.Add(tpoolFee))
	if err != nil {
		return nil, err
	}
	txnBuilder.AddMinerFee(tpoolFee)
	txnBuilder.AddSiacoinOutput(output)
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		return nil, err
	}
	err = w.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		return nil, err
	}
	return txnSet, nil
}
