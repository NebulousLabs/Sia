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
	// RespendTimeout records the number of blocks that the wallet will wait
	// before spending an output that has been spent in the past. If the
	// transaction spending the output has not made it to the transaction pool
	// after the limit, the assumption is that it never will.
	RespendTimeout = 40
)

var (
	errLockedWallet    = errors.New("wallet must be unlocked before it can be used")
	errNilConsensusSet = errors.New("wallet cannot initialize with a nil consensus set")
	errNilTpool        = errors.New("wallet cannot initialize with a nil transaction pool")
)

type spendableKey struct {
	unlockConditions types.UnlockConditions
	secretKeys       []crypto.SecretKey
}

type Wallet struct {
	unlocked    bool
	subscribed  bool
	settings    WalletSettings
	primarySeed modules.Seed

	state              modules.ConsensusSet
	tpool              modules.TransactionPool
	consensusSetHeight types.BlockHeight
	siafundPool        types.Currency

	seeds           []modules.Seed
	keys            map[types.UnlockHash]spendableKey
	siacoinOutputs  map[types.SiacoinOutputID]types.SiacoinOutput
	siafundOutputs  map[types.SiafundOutputID]types.SiafundOutput
	historicOutputs map[types.OutputID]types.Currency
	spentOutputs    map[types.OutputID]types.BlockHeight

	transactions                  map[types.TransactionID]types.Transaction
	walletTransactions            []modules.WalletTransaction
	walletTransactionMap          map[modules.WalletTransactionID]*modules.WalletTransaction
	unconfirmedTransactions       []types.Transaction
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
		return nil, errNilConsensusSet
	}
	if tpool == nil {
		return nil, errNilTpool
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

		transactions:         make(map[types.TransactionID]types.Transaction),
		walletTransactionMap: make(map[modules.WalletTransactionID]*modules.WalletTransaction),

		persistDir: persistDir,
		mu:         sync.New(modules.SafeMutexDelay, 1),
	}

	// Initialize the persistent structures.
	err := w.initPersist()
	if err != nil {
		return nil, err
	}

	return w, nil
}

// Lock will erase all keys from memory and prevent the wallet from spending
// coins until it is unlocked.
func (w *Wallet) Lock() error {
	lockID := w.mu.RLock()
	defer w.mu.RUnlock(lockID)
	w.log.Println("INFO: Closing wallet")

	// Save the wallet data.
	err := w.saveSettings()
	if err != nil {
		return err
	}

	// Wipe all of the secret keys, they will be replaced upon calling 'Unlock'
	// again.
	for _, key := range w.keys {
		// Must use 'for i :=  range' otherwise a copy of the secret data is
		// made.
		for i := range key.secretKeys {
			crypto.SecureWipe(key.secretKeys[i][:])
		}
	}
	w.unlocked = false
	return nil
}

// Unlockd indicates whether the wallet is locked or unlocked.
func (w *Wallet) Unlocked() bool {
	lockID := w.mu.RLock()
	defer w.mu.RUnlock(lockID)
	return w.unlocked
}

// SendSiacoins creates a transaction sending 'amount' to 'dest'. The transaction
// is submitted to the transaction pool and is also returned.
func (w *Wallet) SendSiacoins(amount types.Currency, dest types.UnlockHash) ([]types.Transaction, error) {
	tpoolFee := types.NewCurrency64(10).Mul(types.SiacoinPrecision) // TODO: better fee algo.
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
	tpoolFee := types.NewCurrency64(10).Mul(types.SiacoinPrecision) // TODO: better fee algo.
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
