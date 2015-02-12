package transactionpool

import (
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
)

type unconfirmedTransaction struct {
	transaction  consensus.Transaction
	dependents   map[*unconfirmedTransaction]struct{}

	previous *unconfirmedTransaction
	next     *unconfirmedTransaction
}

type TransactionPool struct {
	state       *consensus.State
	recentBlock consensus.BlockID

	head *unconfirmedTransaction
	tail *unconfirmedTransaction

	siacoinOutputs map[consensus.SiacoinOutputID]consensus.SiacoinOutput
	siafundOutputs map[consensus.SiafundOutputID]consensus.SiafundOutput

	usedSiacoinOutputs       map[consensus.SiacoinOutputID]*unconfirmedTransaction
	fileContractTerminations map[consensus.FileContractID]*unconfirmedTransaction
	storageProofs            map[consensus.BlockID]map[consensus.FileContractID]*unconfirmedTransaction
	usedSiafundOutputs       map[consensus.SiafundOutputID]*unconfirmedTransaction

	mu sync.RWMutex
}

func New(state *consensus.State) (tp *TransactionPool, err error) {
	if state == nil {
		err = errors.New("transaction pool cannot use an nil state")
		return
	}

	tp = &TransactionPool{
		state:       state,
		recentBlock: state.BlockAtHeight(0).ID(),

		siacoinOutputs: make(map[consensus.SiacoinOutputID]consensus.SiacoinOutput),
		siafundOutputs: make(map[consensus.SiafundOutputID]consensus.SiafundOutput),

		usedSiacoinOutputs: make(map[consensus.SiacoinOutputID]*unconfirmedTransaction),
		fileContractTerminations: make(map[consensus.FileContractID]*unconfirmedTransaction),
		storageProofs: make(map[consensus.BlockID]map[consensus.FileContractID]*unconfirmedTransaction),
		usedSiafundOutputs: make(map[consensus.SiafundOutputID]*unconfirmedTransaction),
	}

	tp.state.RLock()
	defer tp.state.RUnlock()
	tp.update()

	return
}

func (tp *TransactionPool) addTransactionToHead(ut *unconfirmedTransaction) {
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
