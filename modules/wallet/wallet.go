package wallet

// TODO: Theoretically, the transaction builder in this wallet supports
// multisig, but there are no automated tests to verify that.

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	siasync "github.com/NebulousLabs/Sia/sync"
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
	errNilConsensusSet = errors.New("wallet cannot initialize with a nil consensus set")
	errNilTpool        = errors.New("wallet cannot initialize with a nil transaction pool")
)

// spendableKey is a set of secret keys plus the corresponding unlock
// conditions.  The public key can be derived from the secret key and then
// matched to the corresponding public keys in the unlock conditions. All
// addresses that are to be used in 'FundSiacoins' or 'FundSiafunds' in the
// transaction builder must conform to this form of spendable key.
type spendableKey struct {
	UnlockConditions types.UnlockConditions
	SecretKeys       []crypto.SecretKey
}

// Wallet is an object that tracks balances, creates keys and addresses,
// manages building and sending transactions.
type Wallet struct {
	// unlocked indicates whether the wallet is currently storing secret keys
	// in memory. subscribed indicates whether the wallet has subscribed to the
	// consensus set yet - the wallet is unable to subscribe to the consensus
	// set until it has been unlocked for the first time. The primary seed is
	// used to generate new addresses for the wallet.
	unlocked    bool
	subscribed  bool
	persist     walletPersist
	primarySeed modules.Seed

	// The wallet's dependencies. The items 'consensusSetHeight' and
	// 'siafundPool' are tracked separately from the consensus set to minimize
	// the number of queries that the wallet needs to make to the consensus
	// set; queries to the consensus set are very slow.
	cs                 modules.ConsensusSet
	tpool              modules.TransactionPool
	consensusSetHeight types.BlockHeight
	siafundPool        types.Currency

	// The following set of fields are responsible for tracking the confirmed
	// outputs, and for being able to spend them. The seeds are used to derive
	// the keys that are tracked on the blockchain. All keys are pregenerated
	// from the seeds, when checking new outputs or spending outputs, the seeds
	// are not referenced at all. The seeds are only stored so that the user
	// may access them.
	//
	// siacoinOutptus, siafundOutputs, and spentOutputs are kept so that they
	// can be scanned when trying to fund transactions.
	seeds          []modules.Seed
	keys           map[types.UnlockHash]spendableKey
	siacoinOutputs map[types.SiacoinOutputID]types.SiacoinOutput
	siafundOutputs map[types.SiafundOutputID]types.SiafundOutput
	spentOutputs   map[types.OutputID]types.BlockHeight

	// The following fields are kept to track transaction history.
	// processedTransactions are stored in chronological order, and have a map for
	// constant time random access. The set of full transactions is kept as
	// well, ordering can be determined by the processedTransactions slice.
	//
	// The unconfirmed transactions are kept the same way, except without the
	// random access. It is assumed that the list of unconfirmed transactions
	// will be small enough that this will not be a problem.
	//
	// historicOutputs is kept so that the values of transaction inputs can be
	// determined. historicOutputs is never cleared, but in general should be
	// small compared to the list of transactions.
	processedTransactions            []modules.ProcessedTransaction
	processedTransactionMap          map[types.TransactionID]*modules.ProcessedTransaction
	unconfirmedProcessedTransactions []modules.ProcessedTransaction

	// The wallet's database tracks its seeds, keys, outputs, and
	// transactions.
	db *persist.BoltDatabase

	persistDir string
	log        *persist.Logger
	mu         sync.RWMutex
	// The wallet's ThreadGroup tells tracked functions to shut down and
	// blocks until they have all exited before returning from Close.
	tg siasync.ThreadGroup
}

// New creates a new wallet, loading any known addresses from the input file
// name and then using the file to save in the future. Keys and addresses are
// not loaded into the wallet during the call to 'new', but rather during the
// call to 'Unlock'.
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
		cs:    cs,
		tpool: tpool,

		keys:           make(map[types.UnlockHash]spendableKey),
		siacoinOutputs: make(map[types.SiacoinOutputID]types.SiacoinOutput),
		siafundOutputs: make(map[types.SiafundOutputID]types.SiafundOutput),
		spentOutputs:   make(map[types.OutputID]types.BlockHeight),

		processedTransactionMap: make(map[types.TransactionID]*modules.ProcessedTransaction),

		persistDir: persistDir,
	}
	err := w.initPersist()
	if err != nil {
		return nil, err
	}
	return w, nil
}

// Close terminates all ongoing processes involving the wallet, enabling
// garbage collection.
func (w *Wallet) Close() error {
	if err := w.tg.Stop(); err != nil {
		return err
	}
	var errs []error
	// Lock the wallet outside of mu.Lock because Lock uses its own mu.Lock.
	// Once the wallet is locked it cannot be unlocked except using the
	// unexported unlock method (w.Unlock returns an error if the wallet's
	// ThreadGroup is stopped).
	if w.Unlocked() {
		if err := w.Lock(); err != nil {
			errs = append(errs, err)
		}
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	w.cs.Unsubscribe(w)
	w.tpool.Unsubscribe(w)

	if err := w.db.Close(); err != nil {
		errs = append(errs, fmt.Errorf("db.Close failed: %v", err))
	}

	if err := w.log.Close(); err != nil {
		errs = append(errs, fmt.Errorf("log.Close failed: %v", err))
	}
	return build.JoinErrors(errs, "; ")
}

// AllAddresses returns all addresses that the wallet is able to spend from,
// including unseeded addresses. Addresses are returned sorted in byte-order.
func (w *Wallet) AllAddresses() []types.UnlockHash {
	w.mu.RLock()
	defer w.mu.RUnlock()

	addrs := make(types.UnlockHashSlice, 0, len(w.keys))
	for addr := range w.keys {
		addrs = append(addrs, addr)
	}
	sort.Sort(addrs)
	return addrs
}
