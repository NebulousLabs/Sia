package miner

import (
	"sync"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/types"
)

type (
	// txnListElemenet is a single element in a txnList
	txnListElement struct {
		txn  *types.Transaction
		prev *txnListElement
		next *txnListElement
	}

	// txnList is a helper structure to allow for quicker lookups, inserts and
	// removals of transactions that should go into a block
	txnList struct {
		first   *txnListElement
		last    *txnListElement
		idToTxn map[types.TransactionID]*txnListElement
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
	txns := []types.Transaction{}
	for element != nil {
		txns = append(txns, *element.txn)
		element = element.next
	}
	return txns
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
