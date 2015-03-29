package transactionpool

import (
	"errors"
	"time"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/sync"
)

// The current transaction pool code is blind to miner fees, and will not
// prioritize transactions that have higher miner fees.

// An unconfirmedTransaction is a node in a linked list containing a
// transaction and pointers to the next and previous transactions in the list.
// The linked list keeps transactions in a specific order, so that transactions
// with dependencies always appear later in the list than their dependencies.
// Linked lists also allow for easy removal of specific elements.
type unconfirmedTransaction struct {
	transaction consensus.Transaction
	previous    *unconfirmedTransaction
	next        *unconfirmedTransaction
}

// The transaction pool keeps an unconfirmed set of transactions along with the
// contracts and outputs that have been created by unconfirmed transactions.
// Incoming transactions are allowed to use objects in the unconfirmed
// consensus set. Doing so will consume them, preventing other transactions
// from using them.
type TransactionPool struct {
	// Depedencies of the transaction pool. The state height is needed
	// separately from the state because the transaction pool may not be
	// synchronized to the state.
	state       *consensus.State
	gateway     modules.Gateway
	stateHeight consensus.BlockHeight

	// A linked list of transactions, with a map pointing to each. Incoming
	// transactions are inserted at the tail if they do not conflict with
	// existing transactions. Transactions pulled from reverted blocks are
	// inserted at the head because there may be dependencies. Inserting in
	// this order ensures that dependencies always appear earlier in the linked
	// list, so a call to TransactionSet() will never dump out-of-order
	// transactions.
	transactions map[crypto.Hash]*unconfirmedTransaction
	head         *unconfirmedTransaction
	tail         *unconfirmedTransaction

	// The unconfirmed set of contracts and outputs. The unconfirmed set
	// includes the confirmed set, except for elements that have been spent by
	// the unconfirmed set.
	siacoinOutputs map[consensus.SiacoinOutputID]consensus.SiacoinOutput
	fileContracts  map[consensus.FileContractID]consensus.FileContract
	siafundOutputs map[consensus.SiafundOutputID]consensus.SiafundOutput

	// The reference set contains any objects that are not in the unconfirmed
	// set, but may still need to be referenced when creating diffs or
	// reverting unconfirmed transactions (due to conflicts).
	referenceSiacoinOutputs map[consensus.SiacoinOutputID]consensus.SiacoinOutput
	referenceFileContracts  map[consensus.FileContractID]consensus.FileContract
	referenceSiafundOutputs map[consensus.SiafundOutputID]consensus.SiafundOutput

	// The entire history of the transaction pool is kept. Each element
	// represents an atomic change to the transaction pool. When a new
	// subscriber joins the transaction pool, they can be sent the entire
	// history and catch up properly, and they can take a long time to catch
	// up. To prevent deadlocks in the transaction pool, subscribers are
	// updated in a separate thread which does not guarantee that a subscriber
	// is always fully synchronized to the transaction pool.
	revertBlocksUpdates     [][]consensus.Block
	applyBlocksUpdates      [][]consensus.Block
	unconfirmedTransactions [][]consensus.Transaction
	unconfirmedSiacoinDiffs [][]consensus.SiacoinOutputDiff
	subscribers             []chan struct{}

	mu *sync.RWMutex
}

// New creates a transaction pool that is ready to receive transactions.
func New(s *consensus.State, g modules.Gateway) (tp *TransactionPool, err error) {
	if s == nil {
		err = errors.New("transaction pool cannot use a nil state")
		return
	}
	if g == nil {
		err = errors.New("transaction pool cannot use a nil gateway")
	}

	// Return a transaction pool with no transactions.
	tp = &TransactionPool{
		state:   s,
		gateway: g,

		transactions:   make(map[crypto.Hash]*unconfirmedTransaction),
		siacoinOutputs: make(map[consensus.SiacoinOutputID]consensus.SiacoinOutput),
		fileContracts:  make(map[consensus.FileContractID]consensus.FileContract),
		siafundOutputs: make(map[consensus.SiafundOutputID]consensus.SiafundOutput),

		referenceSiacoinOutputs: make(map[consensus.SiacoinOutputID]consensus.SiacoinOutput),
		referenceFileContracts:  make(map[consensus.FileContractID]consensus.FileContract),
		referenceSiafundOutputs: make(map[consensus.SiafundOutputID]consensus.SiafundOutput),

		mu: sync.New(1*time.Second, 0),
	}

	s.Subscribe(tp)

	return
}
