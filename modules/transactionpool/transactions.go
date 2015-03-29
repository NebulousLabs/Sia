package transactionpool

import (
	"github.com/NebulousLabs/Sia/consensus"
)

// transactionSet returns the set of unconfirmed transactions in the order
// they are required to appear in a block. This function will not limit the
// volume of transactions to fit in a single block.
func (tp *TransactionPool) transactionSet() (transactionSet []consensus.Transaction) {
	// Iterate through the transactions in the list.
	currentTxn := tp.head
	for currentTxn != nil {
		transactionSet = append(transactionSet, currentTxn.transaction)
		currentTxn = currentTxn.next
	}
	return
}

// TransactionSet returns the set of unconfirmed transactions in the order
// they are required to appear in a block. This function will not limit the
// volume of transactions to fit in a single block.
func (tp *TransactionPool) TransactionSet() []consensus.Transaction {
	id := tp.mu.RLock()
	defer tp.mu.RUnlock(id)
	return tp.transactionSet()
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
