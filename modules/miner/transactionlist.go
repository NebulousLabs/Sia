package miner

import (
	"sync"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/types"
)

type (
	// txnList is a helper structure to allow for quicker lookups, inserts and
	// removals of transactions that should go into a block.
	// In addition to the unsolved block's transaction field, the miner also
	// keeps a txnList in memory. The txnList contains the transactions that
	// will be in the unsolved block the next time blockForWork is called.
	// Whenever we move a transaction from the overflow heap into the block
	// heap, we also append it to the txnList. If we remove a transaction from
	// the block heap we also remove it from the txnList. By using a map to map
	// a transaction id to its element in the list, we can search, insert and
	// remove in constant time. Removing an element from the txnList doesn't
	// change the order of the remaining elements in the list. This is
	// important since we don't want to mess up the relative order of
	// transactions in their transaction sets.
	// Every time blockForWork is called, we copy the transactions from the
	// txnList into a slice that is then assigned to be the unsolved block's
	// updated transaction field. To avoid allocating new memory every time
	// this happens, a preallocated slice is kept in memory. Memory allocation
	// for txnListElements is also optimized by using a memory pool that
	// recycles txnListElements during rapid deletion and insertion.
	txnList struct {
		first            *txnListElement                         // pointer to first element in list
		last             *txnListElement                         // pointer to last element in list
		idToTxn          map[types.TransactionID]*txnListElement // maps transaction ids to list element
		preallocatedTxns []types.Transaction                     // used to return the list's contents without having to reallocate
	}
	// txnListElemenet is a single element in a txnList
	txnListElement struct {
		txn  *types.Transaction
		prev *txnListElement
		next *txnListElement
	}
)

// A pool to reduce the amount of memory allocations when elements are removed
// and inserted rapidly.
var listElementPool = sync.Pool{
	New: func() interface{} {
		return &txnListElement{}
	},
}

// newTxnList creates a new instance of the txnList
func newTxnList() *txnList {
	return &txnList{
		idToTxn: make(map[types.TransactionID]*txnListElement),
	}
}

// newListElement returns a list element with all fields initialized to their
// default values
func newListElement() *txnListElement {
	listElement := listElementPool.Get().(*txnListElement)
	listElement.prev = nil
	listElement.next = nil
	return listElement
}

// appendTxn appends a transaction to the list
func (tl *txnList) appendTxn(txn *types.Transaction) {
	// Create the element and store it in idToTxn for easier lookup by id
	listElement := newListElement()
	listElement.txn = txn
	tl.idToTxn[txn.ID()] = listElement

	// check if it is the first element
	if tl.first == nil {
		tl.first = listElement
		tl.last = listElement
		return
	}
	// if not append it
	tl.last.next = listElement
	listElement.prev = tl.last
	tl.last = listElement
}

// transactions returns the transactions contained in the list as a slice
func (tl *txnList) transactions() []types.Transaction {
	if tl.first == nil {
		return []types.Transaction{}
	}
	element := tl.first
	tl.preallocatedTxns = tl.preallocatedTxns[:0]
	for element != nil {
		tl.preallocatedTxns = append(tl.preallocatedTxns, *element.txn)
		element = element.next
	}
	return tl.preallocatedTxns
}

// removeTxn removes a transaction by id
func (tl *txnList) removeTxn(id types.TransactionID) {
	// Get the corresponding list element and remove it from the map
	listElement, exists := tl.idToTxn[id]
	if !exists {
		build.Critical("transaction is not in the list")
		return
	}
	delete(tl.idToTxn, id)
	defer listElementPool.Put(listElement)

	pe := listElement.prev
	ne := listElement.next
	if pe == nil {
		// listElement is the first element. Set the following element to be
		// the next first element of the list
		tl.first = listElement.next
		// If the new first element is not nil its prev field should be set to nil
		if tl.first != nil {
			tl.first.prev = nil
		}
	}
	if ne == nil {
		// listElement is the last element. Set the previous element to be the
		// next last element of the list and its next field should be nil
		tl.last = listElement.prev
		// If the new last element is not nil its next field should be set to nil
		if tl.last != nil {
			tl.last.next = nil
		}
	}
	// Link pe and ne to each other if they both exist
	if pe != nil && ne != nil {
		pe.next = ne
		ne.prev = pe
	}
}
