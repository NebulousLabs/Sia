package transactionpool

import (
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

// transactionpool.go contains the major objects used when working with the
// transactionpool. The transaction pool needs access to the consensus set and
// to a gateway (to broadcast valid transactions). Transactions are kept in a
// list where new transactions are appended to the end to preserve any
// dependency requirements. Updating the transaction pool happens by removing
// all unconfirmed transactions, adding the changes to the consensus set, and
// then re-adding all of the unconfirmed transactions. Some of the unconfirmed
// transactions may now be invalid, but this will be caught upon re-insertion.
//
// The transaction pool maintains an unconfirmed set and a reference set. The
// unconfirmed set contains all of the elements of the confirmed set except for
// those which have been consumed by unconfirmed transactions, and additionally
// contains any elements that have been added by unconfirmed transactions. The
// reference set contains elements which have been consumed by unconfirmed
// transactions because they might be necessary when constructing diffs.
// Information would otherwise be lost as things get removed from the
// unconfirmed set. The reference set should always be empty when there are no
// unconfirmed transactions.
//
// All changes to the transaction pool are logged by the update set. This is so
// the changes can be sent to subscribers, even subscribers that join late or
// deadlock for some period of time. This could eventually cause performance
// issues, and will be addressed after that becomes a problem.
//
// The transaction pool does not currently prioritize transactions with higher
// fees, and also has no minimum fee. This is a good place to CONTRIBUTE.

// The transaction pool keeps an unconfirmed set of transactions along with the
// contracts and outputs that have been created by unconfirmed transactions.
// Incoming transactions are allowed to use objects in the unconfirmed
// consensus set. Doing so will consume them, preventing other transactions
// from using them.
type TransactionPool struct {
	// Depedencies of the transaction pool. The state height is needed
	// separately from the state because the transaction pool may not be
	// synchronized to the state.
	consensusSet       *consensus.State
	gateway            modules.Gateway
	consensusSetHeight types.BlockHeight

	// A linked list of transactions, with a map pointing to each. Incoming
	// transactions are inserted at the tail if they do not conflict with
	// existing transactions. Transactions pulled from reverted blocks are
	// inserted at the head because there may be dependencies. Inserting in
	// this order ensures that dependencies always appear earlier in the linked
	// list, so a call to TransactionSet() will never dump out-of-order
	// transactions.
	transactions    map[crypto.Hash]struct{}
	transactionList []types.Transaction

	// The unconfirmed set of contracts and outputs. The unconfirmed set
	// includes the confirmed set, except for elements that have been spent by
	// the unconfirmed set.
	siacoinOutputs map[types.SiacoinOutputID]types.SiacoinOutput
	fileContracts  map[types.FileContractID]types.FileContract
	siafundOutputs map[types.SiafundOutputID]types.SiafundOutput

	// The reference set contains any objects that are not in the unconfirmed
	// set, but may still need to be referenced when creating diffs or
	// reverting unconfirmed transactions (due to conflicts).
	referenceSiacoinOutputs        map[types.SiacoinOutputID]types.SiacoinOutput
	referenceFileContracts         map[types.FileContractID]types.FileContract
	referenceFileContractRevisions map[crypto.Hash]types.FileContract
	referenceSiafundOutputs        map[types.SiafundOutputID]types.SiafundOutput

	// The entire history of the transaction pool is kept. Each element
	// represents an atomic change to the transaction pool. When a new
	// subscriber joins the transaction pool, they can be sent the entire
	// history and catch up properly, and they can take a long time to catch
	// up. To prevent deadlocks in the transaction pool, subscribers are
	// updated in a separate thread which does not guarantee that a subscriber
	// is always fully synchronized to the transaction pool.
	consensusChanges        []modules.ConsensusChange
	unconfirmedTransactions [][]types.Transaction
	unconfirmedSiacoinDiffs [][]modules.SiacoinOutputDiff
	subscribers             []chan struct{}

	mu *sync.RWMutex
}

// New creates a transaction pool that is ready to receive transactions.
func New(cs *consensus.State, g modules.Gateway) (tp *TransactionPool, err error) {
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

		transactions:   make(map[crypto.Hash]struct{}),
		siacoinOutputs: make(map[types.SiacoinOutputID]types.SiacoinOutput),
		fileContracts:  make(map[types.FileContractID]types.FileContract),
		siafundOutputs: make(map[types.SiafundOutputID]types.SiafundOutput),

		referenceSiacoinOutputs: make(map[types.SiacoinOutputID]types.SiacoinOutput),
		referenceFileContracts:  make(map[types.FileContractID]types.FileContract),
		referenceSiafundOutputs: make(map[types.SiafundOutputID]types.SiafundOutput),

		mu: sync.New(modules.SafeMutexDelay, 1),
	}

	// Register RPCs
	g.RegisterRPC("RelayTransaction", tp.RelayTransaction)

	// Subscribe the transaction pool to the consensus set.
	cs.ConsensusSetSubscribe(tp)

	return
}
