package transactionpool

import (
	"github.com/NebulousLabs/Sia/consensus"
)

// transactions.go is a temporary file filled with deprecated functions.
// Eventually, all modules dependent on the TransactionSet() function will be
// altered so that they are instead dependent on subscriptions. To my
// knowledge, only siad still needs to be transitioned.

// transactionSet returns the set of unconfirmed transactions in the order
// they are required to appear in a block. This function will not limit the
// volume of transactions to fit in a single block.
func (tp *TransactionPool) transactionSet() (set []consensus.Transaction) {
	for _, txn := range tp.transactionList {
		set = append(set, txn)
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
