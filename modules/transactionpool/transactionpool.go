package transactionpool

import (
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

type ObjectID crypto.Hash
type TransactionSetID crypto.Hash

type TransactionPool struct {
	// Depedencies of the transaction pool. The state height is needed
	// separately from the state because the transaction pool may not be
	// synchronized to the state.
	consensusSet       modules.ConsensusSet
	gateway            modules.Gateway
	consensusSetHeight types.BlockHeight

	// unconfirmedIDs is a set of hashes representing the ID of an object in
	// the unconfirmed set of transactions. Each unconfirmed ID points to the
	// transaciton set containing that object. Transaction sets are sets of
	// transactions that get id'd by their hash. transacitonSetDiffs contain
	// the set of IDs that each transaction set is associated with.
	unconfirmedIDs      map[ObjectID]TransactionSetID
	transactionSets     map[TransactionSetID][]types.Transaction
	transactionSetDiffs map[TransactionSetID][]ObjectID
	// TODO: Write a consistency check comparing transactionSets,
	// transactionSetDiffs.
	//
	// TODO: Write a consistency check making sure that all unconfirmedIDs
	// point to the right place, and that all UnconfirmedIDs are accounted for.

	// The entire history of the transaction pool is kept. Each element
	// represents an atomic change to the transaction pool. When a new
	// subscriber joins the transaction pool, they can be sent the entire
	// history and catch up properly, and they can take a long time to catch
	// up. To prevent deadlocks in the transaction pool, subscribers are
	// updated in a separate thread which does not guarantee that a subscriber
	// is always fully synchronized to the transaction pool.
	consensusChangeIndex    int   // Increments any time a consensus update is made.
	consensusChanges        []int // The index of the consensus change from the consensus set. -1 means there was no change from the consensus set.
	unconfirmedTransactions [][]types.Transaction
	unconfirmedSiacoinDiffs [][]modules.SiacoinOutputDiff
	subscribers             []chan struct{}

	mu *sync.RWMutex
}

// New creates a transaction pool that is ready to receive transactions.
func New(cs modules.ConsensusSet, g modules.Gateway) (tp *TransactionPool, err error) {
	// Check that the input modules are non-nil.
	if cs == nil {
		err = errors.New("transaction pool cannot use a nil state")
		return
	}
	if g == nil {
		err = errors.New("transaction pool cannot use a nil gateway")
		return
	}

	// Initialize a transaction pool.
	tp = &TransactionPool{
		consensusSet: cs,
		gateway:      g,

		unconfirmedIDs:      make(map[ObjectID]TransactionSetID),
		transactionSets:     make(map[TransactionSetID][]types.Transaction),
		transactionSetDiffs: make(map[TransactionSetID][]ObjectID),

		mu: sync.New(modules.SafeMutexDelay, 1),
	}

	// Register RPCs
	g.RegisterRPC("RelayTransaction", tp.RelayTransaction)

	// Subscribe the transaction pool to the consensus set.
	cs.ConsensusSetSubscribe(tp)

	return
}
