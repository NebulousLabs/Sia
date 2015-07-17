package transactionpool

import (
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

type (
	ObjectID         crypto.Hash
	TransactionSetID crypto.Hash

	TransactionPool struct {
		// Depedencies of the transaction pool. The state height is needed
		// separately from the state because the transaction pool may not be
		// synchronized to the state.
		consensusSet modules.ConsensusSet
		gateway      modules.Gateway

		// unconfirmedIDs is a set of hashes representing the ID of an object in
		// the unconfirmed set of transactions. Each unconfirmed ID points to the
		// transaciton set containing that object. Transaction sets are sets of
		// transactions that get id'd by their hash. transacitonSetDiffs contain
		// the set of IDs that each transaction set is associated with.
		knownObjects        map[ObjectID]TransactionSetID
		transactionSets     map[TransactionSetID][]types.Transaction
		transactionSetDiffs map[TransactionSetID]modules.ConsensusChange
		databaseSize        int
		// TODO: Write a consistency check comparing transactionSets,
		// transactionSetDiffs.
		//
		// TODO: Write a consistency check making sure that all unconfirmedIDs
		// point to the right place, and that all UnconfirmedIDs are accounted for.
		//
		// TODO: Need some sort of first-come-first-serve memory.

		// TODO: docstring
		consensusChangeIndex int
		subscribers          []modules.TransactionPoolSubscriber

		mu *sync.RWMutex
	}
)

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

		knownObjects:        make(map[ObjectID]TransactionSetID),
		transactionSets:     make(map[TransactionSetID][]types.Transaction),
		transactionSetDiffs: make(map[TransactionSetID]modules.ConsensusChange),

		// TODO: Docstring
		consensusChangeIndex: -1,

		mu: sync.New(modules.SafeMutexDelay, 1),
	}

	// Register RPCs
	g.RegisterRPC("RelayTransactionSet", tp.RelayTransactionSet)

	// Subscribe the transaction pool to the consensus set.
	cs.ConsensusSetSubscribe(tp)

	return
}

func (tp *TransactionPool) TransactionSet() []types.Transaction {
	var txns []types.Transaction
	for _, tSet := range tp.transactionSets {
		txns = append(txns, tSet...)
	}
	return txns
}
