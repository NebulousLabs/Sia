package transactionpool

import (
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
)

// The current transaction pool code is blind to miner fees, and will not
// prioritize transactions that have higher miner fees.

// An unconfirmedTransaction is a node in a linked list containing a
// transaction and pointers to the next and previous transactions in the list.
type unconfirmedTransaction struct {
	transaction consensus.Transaction
	previous    *unconfirmedTransaction
	next        *unconfirmedTransaction
}

// The TransactionPool keeps a set of transactions that would be valid in a
// block, including transactions that depend on eachother or (in the case of
// storage proofs) depend on a specific block being in the blockchain.
//
// Transactions are kept in a linked list which indicates the order that
// unconfirmed transactions should be added to the blockchain.
type TransactionPool struct {
	state       *consensus.State
	recentBlock consensus.BlockID

	// Linked list variables.
	head *unconfirmedTransaction
	tail *unconfirmedTransaction

	// These maps are essentially equivalent to the unconfirmed consensus set.
	transactions   map[crypto.Hash]struct{}
	siacoinOutputs map[consensus.SiacoinOutputID]consensus.SiacoinOutput
	fileContracts  map[consensus.FileContractID]consensus.FileContract
	siafundOutputs map[consensus.SiafundOutputID]consensus.SiafundOutput

	// These maps point from objects to the unconfirmed transactions that
	// resulted in the objects creation. This is a superset of the unconfirmed
	// consensus set, for example a newFileContract will not necessarily be in
	// the list of fileContracts if an unconfirmed termination has appeared for
	// the unconfirmed file contract.
	usedSiacoinOutputs       map[consensus.SiacoinOutputID]*unconfirmedTransaction
	newSiacoinOutputs        map[consensus.SiacoinOutputID]*unconfirmedTransaction
	newFileContracts         map[consensus.BlockHeight]map[consensus.FileContractID]*unconfirmedTransaction
	fileContractTerminations map[consensus.FileContractID]*unconfirmedTransaction
	storageProofs            map[consensus.BlockID]map[consensus.FileContractID]*unconfirmedTransaction
	usedSiafundOutputs       map[consensus.SiafundOutputID]*unconfirmedTransaction
	newSiafundOutputs        map[consensus.SiafundOutputID]*unconfirmedTransaction

	mu sync.RWMutex
}

// New creates a transaction pool that's ready to receive transactions.
func New(state *consensus.State) (tp *TransactionPool, err error) {
	if state == nil {
		err = errors.New("transaction pool cannot use an nil state")
		return
	}

	// Return a transaction pool with no transactions and a recentBlock
	// pointing to the state's current block.
	tp = &TransactionPool{
		state:       state,
		recentBlock: state.CurrentBlock().ID(),

		siacoinOutputs: make(map[consensus.SiacoinOutputID]consensus.SiacoinOutput),
		siafundOutputs: make(map[consensus.SiafundOutputID]consensus.SiafundOutput),

		usedSiacoinOutputs:       make(map[consensus.SiacoinOutputID]*unconfirmedTransaction),
		fileContractTerminations: make(map[consensus.FileContractID]*unconfirmedTransaction),
		storageProofs:            make(map[consensus.BlockID]map[consensus.FileContractID]*unconfirmedTransaction),
		usedSiafundOutputs:       make(map[consensus.SiafundOutputID]*unconfirmedTransaction),
	}

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
