package transactionpool

import (
	"errors"

	"github.com/NebulousLabs/bolt"
	"github.com/NebulousLabs/demotemutex"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

const (
	dbFilename = "transactionpool.db"
	logFile    = "transactionpool.log"
)

var (
	dbMetadata = persist.Metadata{
		Header:  "Sia Transaction Pool DB",
		Version: "0.6.0",
	}

	errNilCS      = errors.New("transaction pool cannot initialize with a nil consensus set")
	errNilGateway = errors.New("transaction pool cannot initialize with a nil gateway")
)

type (
	// ObjectIDs are the IDs of objects such as siacoin outputs and file
	// contracts, and are used to see if there are conflicts or overlaps within
	// the transaction pool. A TransactionSetID is the hash of a transaction
	// set.
	ObjectID         crypto.Hash
	TransactionSetID crypto.Hash

	// The TransactionPool tracks incoming transactions, accepting them or
	// rejecting them based on internal criteria such as fees and unconfirmed
	// double spends.
	TransactionPool struct {
		// Dependencies of the transaction pool.
		consensusSet modules.ConsensusSet
		gateway      modules.Gateway

		// To prevent double spends in the unconfirmed transaction set, the
		// transaction pool keeps a list of all objects that have either been
		// created or consumed by the current unconfirmed transaction pool. All
		// transactions with overlaps are rejected. This model is
		// over-aggressive - one transaction set may create an object that
		// another transaction set spends. This is done to minimize the
		// computation and memory load on the transaction pool. Dependent
		// transactions should be lumped into a single transaction set.
		//
		// transactionSetDiffs map form a transaction set id to the set of
		// diffs that resulted from the transaction set.
		knownObjects        map[ObjectID]TransactionSetID
		transactionSets     map[TransactionSetID][]types.Transaction
		transactionSetDiffs map[TransactionSetID]modules.ConsensusChange
		transactionListSize int
		// TODO: Write a consistency check comparing transactionSets,
		// transactionSetDiffs.
		//
		// TODO: Write a consistency check making sure that all unconfirmedIDs
		// point to the right place, and that all UnconfirmedIDs are accounted for.

		// The consensus change index tracks how many consensus changes have
		// been sent to the transaction pool. When a new subscriber joins the
		// transaction pool, all prior consensus changes are sent to the new
		// subscriber.
		subscribers []modules.TransactionPoolSubscriber

		// Utilities.
		db         *persist.BoltDatabase
		dbTx       *bolt.Tx
		log        *persist.Logger
		mu         demotemutex.DemoteMutex
		tg         sync.ThreadGroup
		persistDir string
	}
)

// New creates a transaction pool that is ready to receive transactions.
func New(cs modules.ConsensusSet, g modules.Gateway, persistDir string) (*TransactionPool, error) {
	// Check that the input modules are non-nil.
	if cs == nil {
		return nil, errNilCS
	}
	if g == nil {
		return nil, errNilGateway
	}

	// Initialize a transaction pool.
	tp := &TransactionPool{
		consensusSet: cs,
		gateway:      g,

		knownObjects:        make(map[ObjectID]TransactionSetID),
		transactionSets:     make(map[TransactionSetID][]types.Transaction),
		transactionSetDiffs: make(map[TransactionSetID]modules.ConsensusChange),

		persistDir: persistDir,
	}

	// Open the tpool database.
	err := tp.initPersist()
	if err != nil {
		return nil, err
	}

	// Register RPCs
	g.RegisterRPC("RelayTransactionSet", tp.relayTransactionSet)
	tp.tg.OnStop(func() {
		tp.gateway.UnregisterRPC("RelayTransactionSet")
	})
	return tp, nil
}

// Close releases any resources held by the transaction pool, stopping all of
// its worker threads.
func (tp *TransactionPool) Close() error {
	return tp.tg.Stop()
}

// FeeEstimation returns an estimation for what fee should be applied to
// transactions.
func (tp *TransactionPool) FeeEstimation() (min, max types.Currency) {
	// TODO: The fee estimation tool should look at the recent blocks and use
	// them to gauge what sort of fee should be required, as opposed to just
	// guessing blindly.
	//
	// TODO: The current minimum has been reduced significantly to account for
	// legacy renters that are not correctly adding transaction fees. The
	// minimum has been set to 1 siacoin per kb (or 1/1000 SC per byte), but
	// really should look more like 10 SC per kb. But, legacy renters are using
	// a much lower value, which means hosts would be incompatible if the
	// minimum recommended were set to 10. The value has been set to 1, which
	// should be okay temporarily while the renters are given time to upgrade.
	return types.SiacoinPrecision.Mul64(1).Div64(20).Div64(1e3), types.SiacoinPrecision.Mul64(1).Div64(1e3) // TODO: Adjust down once miners have upgraded.
}

// TransactionList returns a list of all transactions in the transaction pool.
// The transactions are provided in an order that can acceptably be put into a
// block.
func (tp *TransactionPool) TransactionList() []types.Transaction {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	var txns []types.Transaction
	for _, tSet := range tp.transactionSets {
		txns = append(txns, tSet...)
	}
	return txns
}
