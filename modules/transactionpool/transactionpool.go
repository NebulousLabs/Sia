package transactionpool

import (
	"errors"

	"github.com/NebulousLabs/demotemutex"
	"github.com/coreos/bbolt"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

var (
	errNilCS      = errors.New("transaction pool cannot initialize with a nil consensus set")
	errNilGateway = errors.New("transaction pool cannot initialize with a nil gateway")
)

type (
	// ObjectID is the ID of an object such as siacoin output and file
	// contracts, and is used to see if there is are conflicts or overlaps within
	// the transaction pool.
	ObjectID crypto.Hash
	// TransactionSetID is the hash of a transaction set.
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
		subscriberSets      map[TransactionSetID]*modules.UnconfirmedTransactionSet
		transactionHeights  map[types.TransactionID]types.BlockHeight
		transactionSets     map[TransactionSetID][]types.Transaction
		transactionSetDiffs map[TransactionSetID]*modules.ConsensusChange
		transactionListSize int

		// Variables related to the blockchain.
		blockHeight     types.BlockHeight
		recentMedians   []types.Currency
		recentMedianFee types.Currency // SC per byte

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
		subscriberSets:      make(map[TransactionSetID]*modules.UnconfirmedTransactionSet),
		transactionHeights:  make(map[types.TransactionID]types.BlockHeight),
		transactionSets:     make(map[TransactionSetID][]types.Transaction),
		transactionSetDiffs: make(map[TransactionSetID]*modules.ConsensusChange),

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
// transactions. It returns a minimum and maximum estimated fee per transaction
// byte.
func (tp *TransactionPool) FeeEstimation() (min, max types.Currency) {
	err := tp.tg.Add()
	if err != nil {
		return
	}
	defer tp.tg.Done()
	tp.mu.Lock()
	defer tp.mu.Unlock()

	// Use three methods to determine an acceptable fee, and then take the
	// largest result of the two methods. The first method checks the historic
	// blocks, to make sure that we don't under-estimate the number of fees
	// needed in the event that we just purged the tpool.
	//
	// The second method looks at the existing tpool. Sudden congestion won't be
	// represented on the blockchain right away, but should be immediately
	// influencing how you set fees. Using the current tpool fullness will help
	// pick appropriate fees in the event of sudden congestion.
	//
	// The third method just has hardcoded minimums as a sanity check. In the
	// event of empty blocks, there should still be some fees being added to the
	// chain.

	// Set the minimum fee to the numbers recommended by the blockchain.
	min = tp.recentMedianFee
	max = tp.recentMedianFee.Mul64(maxMultiplier)

	// Method two: use 'requiredFeesToExtendPool'.
	required := tp.requiredFeesToExtendTpool()
	requiredMin := required.MulFloat(minExtendMultiplier) // Clear the local requirement by a little bit.
	requiredMax := requiredMin.MulFloat(maxMultiplier)    // Clear the local requirement by a lot.
	if min.Cmp(requiredMin) < 0 {
		min = requiredMin
	}
	if max.Cmp(requiredMax) < 0 {
		max = requiredMax
	}

	// Method three: sane mimimums.
	if min.Cmp(minEstimation) < 0 {
		min = minEstimation
	}
	if max.Cmp(minEstimation.Mul64(maxMultiplier)) < 0 {
		max = minEstimation.Mul64(maxMultiplier)
	}

	return
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

// Transaction returns the transaction with the provided txid, its parents, and
// a bool indicating if it exists in the transaction pool.
func (tp *TransactionPool) Transaction(id types.TransactionID) (types.Transaction, []types.Transaction, bool) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	// find the transaction
	exists := false
	var txn types.Transaction
	var allParents []types.Transaction
	for _, tSet := range tp.transactionSets {
		for i, t := range tSet {
			if t.ID() == id {
				txn = t
				allParents = tSet[:i]
				exists = true
				break
			}
		}
	}

	// prune unneeded parents
	parentIDs := make(map[types.OutputID]struct{})
	addOutputIDs := func(txn types.Transaction) {
		for _, input := range txn.SiacoinInputs {
			parentIDs[types.OutputID(input.ParentID)] = struct{}{}
		}
		for _, fcr := range txn.FileContractRevisions {
			parentIDs[types.OutputID(fcr.ParentID)] = struct{}{}
		}
		for _, input := range txn.SiafundInputs {
			parentIDs[types.OutputID(input.ParentID)] = struct{}{}
		}
		for _, proof := range txn.StorageProofs {
			parentIDs[types.OutputID(proof.ParentID)] = struct{}{}
		}
		for _, sig := range txn.TransactionSignatures {
			parentIDs[types.OutputID(sig.ParentID)] = struct{}{}
		}
	}
	isParent := func(t types.Transaction) bool {
		for i := range t.SiacoinOutputs {
			if _, exists := parentIDs[types.OutputID(t.SiacoinOutputID(uint64(i)))]; exists {
				return true
			}
		}
		for i := range t.FileContracts {
			if _, exists := parentIDs[types.OutputID(t.SiacoinOutputID(uint64(i)))]; exists {
				return true
			}
		}
		for i := range t.SiafundOutputs {
			if _, exists := parentIDs[types.OutputID(t.SiacoinOutputID(uint64(i)))]; exists {
				return true
			}
		}
		return false
	}

	addOutputIDs(txn)
	var necessaryParents []types.Transaction
	for i := len(allParents) - 1; i >= 0; i-- {
		parent := allParents[i]

		if isParent(parent) {
			necessaryParents = append([]types.Transaction{parent}, necessaryParents...)
			addOutputIDs(parent)
		}
	}

	return txn, necessaryParents, exists
}

// TransactionSet returns the transaction set the provided object
// appears in.
func (tp *TransactionPool) TransactionSet(oid crypto.Hash) []types.Transaction {
	tp.mu.RLock()
	defer tp.mu.RUnlock()
	var parents []types.Transaction
	tSetID, exists := tp.knownObjects[ObjectID(oid)]
	if !exists {
		return nil
	}
	tSet, exists := tp.transactionSets[tSetID]
	if !exists {
		return nil
	}
	parents = append(parents, tSet...)
	return parents
}

// Broadcast broadcasts a transaction set to all of the transaction pool's
// peers.
func (tp *TransactionPool) Broadcast(ts []types.Transaction) {
	go tp.gateway.Broadcast("RelayTransactionSet", ts, tp.gateway.Peers())
}
