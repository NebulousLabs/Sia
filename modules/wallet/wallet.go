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
	unlockConditions types.UnlockConditions
	secretKeys       []crypto.SecretKey
}

type Wallet struct {
	unlocked    bool
	settings    WalletSettings
	primarySeed modules.Seed

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

	walletTransactions            []modules.WalletTransaction
	walletTransactionMap          map[modules.WalletTransactionID]*modules.WalletTransaction
	unconfirmedWalletTransactions []modules.WalletTransaction

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

		keys:            make(map[types.UnlockHash]spendableKey),
		siacoinOutputs:  make(map[types.SiacoinOutputID]types.SiacoinOutput),
		siafundOutputs:  make(map[types.SiafundOutputID]types.SiafundOutput),
		historicOutputs: make(map[types.OutputID]types.Currency),
		spentOutputs:    make(map[types.OutputID]types.BlockHeight),

		walletTransactionMap: make(map[modules.WalletTransactionID]*modules.WalletTransaction),

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
	return w.saveSettings()
}

// SendSiacoins creates a transaction sending 'amount' to 'dest'. The transaction
// is submitted to the transaction pool and is also returned.
func (w *Wallet) SendSiacoins(amount types.Currency, dest types.UnlockHash) ([]types.Transaction, error) {
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

// SendSiafunds creates a transaction sending 'amount' to 'dest'. The transaction
// is submitted to the transaction pool and is also returned.
func (w *Wallet) SendSiafunds(amount types.Currency, dest types.UnlockHash) ([]types.Transaction, error) {
	tpoolFee := types.NewCurrency64(10).Mul(types.SiacoinPrecision)
	output := types.SiafundOutput{
		Value:      amount,
		UnlockHash: dest,
	}

	txnBuilder := w.StartTransaction()
	err := txnBuilder.FundSiacoins(tpoolFee)
	if err != nil {
		return nil, err
	}
	err = txnBuilder.FundSiafunds(amount)
	if err != nil {
		return nil, err
	}
	txnBuilder.AddMinerFee(tpoolFee)
	txnBuilder.AddSiafundOutput(output)
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
