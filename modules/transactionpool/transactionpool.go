package transactionpool

import (
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/hash"
)

// An unconfirmedTransaction contains a transaction that hasn't been confirmed
// by the blockchain yet. It also points to the unconfirmedTransactions that it
// depends on, and the unconfirmedTransactions that depend on it.
//
// An unconfirmedTransaction is additionally a member of a doubly linked list.
// New transactions are added to the tail of this list, while transactions that
// got pulled from the blockchain during a reorg are added to the beginning of
// the list.
type unconfirmedTransaction struct {
	transaction  consensus.Transaction
	requirements []*unconfirmedTransaction
	dependents   []*unconfirmedTransaction

	previous *unconfirmedTransaction
	next     *unconfirmedTransaction
}

// The TransactionPool contains a list of transactions that have not yet been
// confirmed by a block. Transactions with storage proofs are handled
// separately for reasons discussed in Standard.md
type TransactionPool struct {
	state *consensus.State

	// The head and tail of the linked list of transactions that can be put
	// into blocks.
	head *unconfirmedTransaction
	tail *unconfirmedTransaction

	// Outputs contains a list of outputs that have been created by unconfirmed
	// transactions. This list will not include outputs created by storage
	// proofs.
	outputs map[consensus.OutputID]consensus.Output

	// newOutputs is a mapping from an OutputID to the unconfirmed transaction
	// that created the output. usedOutputs is a mapping to the unconfirmed
	// transaction that used the output. These mappings are useful for
	// determining dependencies and properly reorganizing the transaction pool
	// in the even that a double spend makes it into the blockchain.
	newOutputs  map[consensus.OutputID]*unconfirmedTransaction
	usedOutputs map[consensus.OutputID]*unconfirmedTransaction

	// storageProofs is a list of transactions that contain storage proofs
	// sorted by the height of the highest start point of a storage proof. This
	// is useful for determining which storage proof transactions should be put
	// into the transaction pool dump depending on the current organization of
	// the blockchain.
	storageProofs map[consensus.BlockHeight]map[hash.Hash]consensus.Transaction

	mu sync.RWMutex
}

// addTransactionToTail takes an unconfirmedTransaction and adds it to the tail
// of the linked list of transactions.
func (tp *TransactionPool) addTransactionToTail(ut *unconfirmedTransaction) {
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

func (tp *TransactionPool) removeTransactionFromList(ut *unconfirmedTransaction) {
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
