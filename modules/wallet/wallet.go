package wallet

// TODO: Theoretically, the transaction builder in this wallet supports
// multisig, but there are no automated tests to verify that.

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"sync"

	"github.com/NebulousLabs/bolt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	siasync "github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
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
	// encrypted indicates whether the wallet has been encrypted (i.e.
	// initialized). unlocked indicates whether the wallet is currently
	// storing secret keys in memory. subscribed indicates whether the wallet
	// has subscribed to the consensus set yet - the wallet is unable to
	// subscribe to the consensus set until it has been unlocked for the first
	// time. The primary seed is used to generate new addresses for the
	// wallet.
	encrypted   bool
	unlocked    bool
	subscribed  bool
	primarySeed modules.Seed

	// The wallet's dependencies.
	cs    modules.ConsensusSet
	tpool modules.TransactionPool
	deps  Dependencies

	// The following set of fields are responsible for tracking the confirmed
	// outputs, and for being able to spend them. The seeds are used to derive
	// the keys that are tracked on the blockchain. All keys are pregenerated
	// from the seeds, when checking new outputs or spending outputs, the seeds
	// are not referenced at all. The seeds are only stored so that the user
	// may access them.
	seeds     []modules.Seed
	keys      map[types.UnlockHash]spendableKey
	lookahead map[types.UnlockHash]uint64

	// unconfirmedProcessedTransactions tracks unconfirmed transactions.
	//
	// TODO: Replace this field with a linked list. Currently when a new
	// transaction set diff is provided, the entire array needs to be
	// reallocated. Since this can happen tens of times per second, and the
	// array can have tens of thousands of elements, it's a performance issue.
	unconfirmedSets                  map[modules.TransactionSetID][]types.TransactionID
	unconfirmedProcessedTransactions []modules.ProcessedTransaction

	// The wallet's database tracks its seeds, keys, outputs, and
	// transactions. A global db transaction is maintained in memory to avoid
	// excessive disk writes. Any operations involving dbTx must hold an
	// exclusive lock.
	db   *persist.BoltDatabase
	dbTx *bolt.Tx

	persistDir string
	log        *persist.Logger
	mu         sync.RWMutex

	// A separate TryMutex is used to protect against concurrent unlocking or
	// initialization.
	scanLock siasync.TryMutex

	// The wallet's ThreadGroup tells tracked functions to shut down and
	// blocks until they have all exited before returning from Close.
	tg siasync.ThreadGroup

	// defragDisabled determines if the wallet is set to defrag outputs once it
	// reaches a certain threshold
	defragDisabled bool
}

func (w *Wallet) Height() types.BlockHeight {
	w.mu.Lock()
	defer w.mu.Unlock()

	var height uint64
	err := w.db.View(func(tx *bolt.Tx) error {
		return encoding.Unmarshal(tx.Bucket(bucketWallet).Get(keyConsensusHeight), &height)
	})
	if err != nil {
		log.Println(err)
		return types.BlockHeight(0)
	}
	return types.BlockHeight(height)
}

// commitTransactionSet is a convenience wrapper for the transaction pools
// AcceptTransactionSet method. It only returns an error if the transaction was
// rejected and won't be rebroadcasted over time
func (w *Wallet) commitTransactionSet(txns []types.Transaction) error {
	w.mu.Unlock()
	err := w.tpool.AcceptTransactionSet(txns)
	w.mu.Lock()
	if err == nil {
		// If we were able to add the transactions to the pool we are done. The
		// wallet already updated the unconfirmedSets and
		// unconfirmedProcessedTransactions fields in
		// ReceiveUpdatedUnconfirmedTransactions
		return nil
	}
	// If there was a consensus conflict we shouldn't add the set
	if cconflict, ok := err.(modules.ConsensusConflict); ok {
		return cconflict
	}
	// If the set was already added we don't need to add it again
	if err == modules.ErrDuplicateTransactionSet {
		return err
	}
	// TODO: There might be more errors that make us abort here

	// If we couldn't add the transaction but still want the wallet to track it
	// we need to add it manually to the unconfirmedSets and
	// unconfirmedProcessedTransactions
	tSetID := modules.TransactionSetID(crypto.HashObject(txns))
	ids := make([]types.TransactionID, 0, len(txns))
	pts := make([]modules.ProcessedTransaction, 0, len(txns))
	for _, txn := range txns {
		ids = append(ids, txn.ID())
		pt := modules.ProcessedTransaction{
			Transaction:           txn,
			TransactionID:         txn.ID(),
			ConfirmationHeight:    types.BlockHeight(math.MaxUint64),
			ConfirmationTimestamp: types.Timestamp(math.MaxUint64),
		}
		// TODO Also add processed inputs and outputs
		pts = append(pts, pt)
	}
	// Add the unconfirmed set
	w.unconfirmedSets[tSetID] = ids
	if err := dbPutUnconfirmedSet(w.dbTx, tSetID, ids); err != nil {
		return err
	}
	// Add the unconfirmed processed transactions
	w.unconfirmedProcessedTransactions = append(w.unconfirmedProcessedTransactions, pts...)
	return nil
}

// managedCommitTransactionSet is a thread-safe version of commitTransactionSet
func (w *Wallet) managedCommitTransactionSet(txns []types.Transaction) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.commitTransactionSet(txns)
}

// New creates a new wallet, loading any known addresses from the input file
// name and then using the file to save in the future. Keys and addresses are
// not loaded into the wallet during the call to 'new', but rather during the
// call to 'Unlock'.
func New(cs modules.ConsensusSet, tpool modules.TransactionPool, persistDir string) (*Wallet, error) {
	return newWallet(cs, tpool, persistDir, &ProductionDependencies{})
}

func newWallet(cs modules.ConsensusSet, tpool modules.TransactionPool, persistDir string, deps Dependencies) (*Wallet, error) {
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

		keys:      make(map[types.UnlockHash]spendableKey),
		lookahead: make(map[types.UnlockHash]uint64),

		unconfirmedSets: make(map[modules.TransactionSetID][]types.TransactionID),

		persistDir: persistDir,

		deps: deps,
	}
	err := w.initPersist()
	if err != nil {
		return nil, err
	}

	// begin the initial transaction
	w.dbTx, err = w.db.Begin(true)
	if err != nil {
		w.log.Critical("ERROR: failed to start database update:", err)
	}

	// COMPATv131 we need to create the bucketProcessedTxnIndex if it doesn't exist
	if w.dbTx.Bucket(bucketProcessedTransactions).Stats().KeyN > 0 &&
		w.dbTx.Bucket(bucketProcessedTxnIndex).Stats().KeyN == 0 {
		err = initProcessedTxnIndex(w.dbTx)
		if err != nil {
			return nil, err
		}
		// Save changes to disk
		w.syncDB()
	}

	// Load possible unconfirmed sets from disk
	w.unconfirmedSets, err = dbLoadUnconfirmedSets(w.dbTx)
	if err != nil {
		return nil, err
	}

	// make sure we commit on shutdown
	w.tg.AfterStop(func() {
		err := w.dbTx.Commit()
		if err != nil {
			w.log.Println("ERROR: failed to apply database update:", err)
			w.dbTx.Rollback()
		}
	})
	go w.threadedDBUpdate()

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

	w.cs.Unsubscribe(w)
	w.tpool.Unsubscribe(w)

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

	addrs := make([]types.UnlockHash, 0, len(w.keys))
	for addr := range w.keys {
		addrs = append(addrs, addr)
	}
	sort.Slice(addrs, func(i, j int) bool {
		return bytes.Compare(addrs[i][:], addrs[j][:]) < 0
	})
	return addrs
}

// Rescanning reports whether the wallet is currently rescanning the
// blockchain.
func (w *Wallet) Rescanning() bool {
	rescanning := !w.scanLock.TryLock()
	if !rescanning {
		w.scanLock.Unlock()
	}
	return rescanning
}

// Returns the wallet's current settings
func (w *Wallet) Settings() modules.WalletSettings {
	return modules.WalletSettings{
		NoDefrag: w.defragDisabled,
	}
}

// SetSettings will update the settings for the wallet.
func (w *Wallet) SetSettings(s modules.WalletSettings) {
	w.defragDisabled = s.NoDefrag
}
