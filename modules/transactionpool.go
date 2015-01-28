package modules

import (
	"github.com/NebulousLabs/Sia/consensus"
)

type TransactionPool interface {
	// AcceptTransaction takes a transaction, analyzes it, and either rejects
	// it or adds it to the transaction pool, returning an error if the
	// transaction is rejected.
	AcceptTransaction(consensus.Transaction) error

	// TransactionSet will return a set of transactions not exceeding the block
	// size that can be inserted into a block in order.
	TransactionSet() ([]consensus.Transaction, error)
}
