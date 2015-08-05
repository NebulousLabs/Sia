package wallet

import (
	"errors"
	"log"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// TransactionFee is yet another deprecated-on-arrival constant that says
	// how large the transaction fees should be. This should really be a
	// function supplied by the transaction pool.
	TransactionFee = 10

	// RespendTimeout records the number of blocks that the wallet will wait
	// before spending an output that has been spent in the past. If the
	// transaction spending the output has not made it to the transaction pool
	// after the limit, the assumption is that it never will.
	RespendTimeout = 40
)

var (
	errLockedWallet = errors.New("wallet must be unlocked before it can be used")
)

type spendableKey struct {
	unlockConditions UnlockConditions
	secretKeys       []crypto.SecretKey
}

type Wallet struct {
	unlocked    bool
	settings    WalletSettings
	primarySeed Seed

	state                   modules.ConsensusSet
	tpool                   modules.TransactionPool
	consensusSetHeight      types.BlockHeight
	siafundPool             types.Currency
	unconfirmedTransactions []types.Transaction

	keys            map[types.UnlockHash]spendableKey
	siacoinOutputs  map[types.SiacoinOutputID]types.SiacoinOutput
	siafundOutputs  map[types.SiafundOutputID]types.SiafundOutput
	historicOutputs map[types.OutputID]types.Currency
	spentOutputs    map[types.OutputID]types.BlockHeight

	walletTransactions            []WalletTransaction // A doubly linked list would be safer when adding and removing items.
	walletTransactionMap          map[WalletTransactionID]*WalletTransaction
	unconfirmedWalletTransactions []WalletTransaction // no map, just iterate through the whole thing

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

		generatedKeys: make(map[types.UnlockHash]generatedSignatureKey),
		trackedKeys:   make(map[types.UnlockHash]struct{}),

		persistDir: persistDir,
		mu:         sync.New(modules.SafeMutexDelay, 1),
	}

	// Initialize the persistent structures.
	err := w.initPersist()
	if err != nil {
		return nil, err
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
