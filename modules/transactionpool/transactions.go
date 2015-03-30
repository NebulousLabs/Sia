package transactionpool

import (
	"github.com/NebulousLabs/Sia/consensus"
)

// transactionSet returns the set of unconfirmed transactions in the order
// they are required to appear in a block. This function will not limit the
// volume of transactions to fit in a single block.
func (tp *TransactionPool) transactionSet() (transactionSet []consensus.Transaction) {
	// Iterate through the transactions in the list.
	for _, txn := range tp.transactionList {
		transactionSet = append(transactionSet, *txn)
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
