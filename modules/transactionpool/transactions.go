package transactionpool

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
)

func (tp *TransactionPool) fullTransactionSet() (transactionSet []consensus.Transaction) {
	// Iterate through the transactions in the list.
	currentTxn := tp.head
	for currentTxn != nil {
		transactionSet = append(transactionSet, currentTxn.transaction)
		currentTxn = currentTxn.next
	}
	return
}

// FullTransactionSet returns the list of all transactions in the transaction
// pool, in the order that they could be put into blocks. FullTransactionSet
// will return all of the transactions, and not just the ones that will fit
// into a block.
func (tp *TransactionPool) FullTransactionSet() []consensus.Transaction {
	id := tp.mu.RLock()
	defer tp.mu.RUnlock(id)
	return tp.fullTransactionSet()
}

// TransactionSet will return a list of transactions that can be put in a block
// in order, and will not result in the block being too large. TransactionSet
// prioritizes transactions that have already been in a block (on another
// fork), and then treats remaining transactions in a first come first serve
// manner.
func (tp *TransactionPool) TransactionSet() (transactionSet []consensus.Transaction) {
	id := tp.mu.RLock()
	defer tp.mu.RUnlock(id)

	// Add transactions from the head of the linked list until there are no
	// more transactions or until the size limit has been reached.
	var remainingSize int = consensus.BlockSizeLimit - 5e3 // Leave 5kb for the block header and the miner transaction.

	// Iterate through the transactions and add them in first-come-first-serve
	// order.
	currentTxn := tp.head
	for currentTxn != nil {
		// Allocate space for the transaction, exiting the loop if there is not
		// enough space.
		encodedTxn := encoding.Marshal(currentTxn.transaction)
		remainingSize -= len(encodedTxn)
		if remainingSize < 0 {
			break
		}

		// Add the transaction to the transaction set and move onto the next
		// transaction.
		transactionSet = append(transactionSet, currentTxn.transaction)
		currentTxn = currentTxn.next
	}

	return
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
