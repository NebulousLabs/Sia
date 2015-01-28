package modules

import (
	"github.com/NebulousLabs/Sia/consensus"
)

type TransactionPool interface {
	// AcceptTransaction takes a transaction, analyzes it, and either rejects
	// it or adds it to the transaction pool, returning an error if the
	// transaction is rejected.
	AcceptTransaction(consensus.Transaction) error

	// OutputDiffs returns the set of diffs that are in the transaction pool
	// but haven't been confirmed by a block yet.
	OutputDiffs() []consensus.OutputDiff

	// TransactionSet will return a set of transactions not exceeding the block
	// size that can be inserted into a block in order.
	//
	// TransactionSet has special behavior, it will always update with the
	// state before returning anything. A function can safely update from both
	// the transaction pool and the state by read locking the state, then
	// updating from the transaction pool, then the state, and then unlocking
	// the state.
	TransactionSet() ([]consensus.Transaction, error)
}
