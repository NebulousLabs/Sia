package transactionpool

import (
	"errors"

	"github.com/NebulousLabs/demotemutex"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
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
		// Depedencies of the transaction pool.
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
		consensusChangeIndex int
		subscribers          []modules.TransactionPoolSubscriber

		mu demotemutex.DemoteMutex
	}
)

// New creates a transaction pool that is ready to receive transactions.
func New(cs modules.ConsensusSet, g modules.Gateway) (*TransactionPool, error) {
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

		// The consensus change index is intialized to '-1', which indicates
		// that no consensus changes have been sent yet. The first consensus
		// change will then have an index of '0'.
		consensusChangeIndex: -1,
	}
	// Register RPCs
	// TODO: rename RelayTransactionSet so that the conflicting RPC
	// RelayTransaction calls v0.4.6 clients and earlier are ignored.
	g.RegisterRPC("RelayTransactionSet", tp.relayTransactionSet)

	// Subscribe the transaction pool to the consensus set.
	err := cs.ConsensusSetPersistentSubscribe(tp, modules.ConsensusChangeID{})
	if err != nil {
		return nil, errors.New("transactionpool subscription failed: " + err.Error())
	}

	return tp, nil
}

// FeeEstimation returns an estimation for what fee should be applied to
// transactions.
func (tp *TransactionPool) FeeEstimation() (min, max types.Currency) {
	// TODO: The fee estimation tool should look at the recent blocks and use
	// them to guage what sort of fee should be required, as opposed to just
	// guessing blindly.
	return types.NewCurrency64(2), types.NewCurrency64(3)
}

// TransactionList returns a list of all transactions in the transaction pool.
// The transactions are provided in an order that can acceptably be put into a
// block.
func (tp *TransactionPool) TransactionList() []types.Transaction {
	var txns []types.Transaction
	for _, tSet := range tp.transactionSets {
		txns = append(txns, tSet...)
	}
	return txns
}
