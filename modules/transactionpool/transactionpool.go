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
// consensus set, and in doing so will consume them, preventing other
// transactions from using them.
//
// Then there are a set of maps indicating which unconfirmed transactions have
// created or consumed objects in the consensus set. This helps with detecting
// invalid transactions, and transactions that are in conflict with other
// unconfirmed transactions.
type TransactionPool struct {
	state       *consensus.State
	gateway     modules.Gateway
	stateHeight consensus.BlockHeight

	// Linked list variables.
	head *unconfirmedTransaction
	tail *unconfirmedTransaction

	// These maps are essentially equivalent to the unconfirmed consensus set.
	transactions   map[crypto.Hash]*unconfirmedTransaction
	siacoinOutputs map[consensus.SiacoinOutputID]consensus.SiacoinOutput
	fileContracts  map[consensus.FileContractID]consensus.FileContract
	siafundOutputs map[consensus.SiafundOutputID]consensus.SiafundOutput

	// These maps point from objects to the unconfirmed transactions that
	// resulted in the objects creation. This is a superset of the unconfirmed
	// consensus set, for example a newFileContract will not necessarily be in
	// the list of fileContracts if an unconfirmed termination has appeared for
	// the unconfirmed file contract.
	usedSiacoinOutputs       map[consensus.SiacoinOutputID]*unconfirmedTransaction
	newFileContracts         map[consensus.BlockHeight]map[consensus.FileContractID]*unconfirmedTransaction
	fileContractTerminations map[consensus.FileContractID]*unconfirmedTransaction
	storageProofs            map[consensus.BlockID]map[consensus.FileContractID]*unconfirmedTransaction
	usedSiafundOutputs       map[consensus.SiafundOutputID]*unconfirmedTransaction

	// Subscriber variables
	revertBlocksUpdates [][]consensus.Block
	applyBlocksUpdates  [][]consensus.Block
	revertTxnsUpdates   [][]consensus.Transaction
	applyTxnsUpdates    [][]consensus.Transaction
	subscribers         []chan struct{}

	mu *sync.RWMutex
}

// New creates a transaction pool that's ready to receive transactions.
func New(s *consensus.State, g modules.Gateway) (tp *TransactionPool, err error) {
	if s == nil {
		err = errors.New("transaction pool cannot use a nil state")
		return
	}
	if g == nil {
		err = errors.New("transaction pool cannot use a nil gateway")
	}

	// Return a transaction pool with no transactions and a recentBlock
	// pointing to the state's current block.
	tp = &TransactionPool{
		state:   s,
		gateway: g,

		transactions:   make(map[crypto.Hash]*unconfirmedTransaction),
		siacoinOutputs: make(map[consensus.SiacoinOutputID]consensus.SiacoinOutput),
		fileContracts:  make(map[consensus.FileContractID]consensus.FileContract),
		siafundOutputs: make(map[consensus.SiafundOutputID]consensus.SiafundOutput),

		usedSiacoinOutputs:       make(map[consensus.SiacoinOutputID]*unconfirmedTransaction),
		newFileContracts:         make(map[consensus.BlockHeight]map[consensus.FileContractID]*unconfirmedTransaction),
		fileContractTerminations: make(map[consensus.FileContractID]*unconfirmedTransaction),
		storageProofs:            make(map[consensus.BlockID]map[consensus.FileContractID]*unconfirmedTransaction),
		usedSiafundOutputs:       make(map[consensus.SiafundOutputID]*unconfirmedTransaction),

		seenTransactions: make(map[crypto.Hash]struct{}),

		mu: sync.New(1*time.Second, 0),
	}

	s.Subscribe(tp)

	return
}

// prependUnconfirmedTransaction takes an unconfirmed transaction and prepends
// it to the linked list of the transaction pool.
func (tp *TransactionPool) prependUnconfirmedTransaction(ut *unconfirmedTransaction) {
	if tp.head == nil {
		// Sanity check - tail should never be nil unless head is also nil.
		if consensus.DEBUG {
			if tp.tail != nil {
				panic("head is nil but tail is not nil")
			}
		}

		tp.head = ut
		tp.tail = ut
	} else {
		tp.head.previous = ut
		ut.next = tp.head
		tp.head = ut
	}
}

// appendUnconfirmedTransaction takes an unconfirmed transaction and appends it
// to the linked list of the transaction pool.
func (tp *TransactionPool) appendUnconfirmedTransaction(ut *unconfirmedTransaction) {
	// Add the unconfirmedTransaction to the tail of the linked list.
	if tp.tail == nil {
		// Sanity check - tail should never be nil unless head is also nil.
		if consensus.DEBUG {
			if tp.head != nil {
				panic("tail is nil but head is not nil")
			}
		}

		tp.head = ut
		tp.tail = ut
	} else {
		tp.tail.next = ut
		ut.previous = tp.tail
		tp.tail = ut
	}
}

// removeUnconfirmedTransactionFromList removes an unconfirmed transaction from
// the linked list of the transaction pool. It does not update any of the other
// fields of the transaction pool.
func (tp *TransactionPool) removeUnconfirmedTransactionFromList(ut *unconfirmedTransaction) {
	// Point the previous unconfirmed transaction at the next unconfirmed
	// transaction.
	if ut.previous == nil {
		// Sanity check - ut should be the head if ut.previous is nil.
		if consensus.DEBUG {
			if tp.head != ut {
				panic("ut.previous is nil but tp.head is not ut")
			}
		}

		tp.head = ut.next
	} else {
		ut.previous.next = ut.next
	}

	// Point the next unconfirmed transaction at the previous unconfirmed
	// transaction.
	if ut.next == nil {
		// Sanity check - ut should be the tail if ut.next is nil.
		if consensus.DEBUG {
			if tp.tail != ut {
				panic("ut.next is nil but tp.tail is not ut")
			}
		}

		tp.tail = ut.previous
	} else {
		ut.next.previous = ut.previous
	}

	// Sanity check - if head or tail is nil, both should be nil.
	if consensus.DEBUG {
		if (tp.head == nil || tp.tail == nil) && (tp.head != nil || tp.tail != nil) {
			panic("one of tp.head and tp.tail is nil, but the other is not")
		}
	}
}
