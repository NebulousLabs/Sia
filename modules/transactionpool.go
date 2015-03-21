package modules

import (
	"github.com/NebulousLabs/Sia/consensus"
)

// A TransactionPoolSubscriber receives updates about the confirmed and
// unconfirmed set from the transaction pool. Generally, there is no need to
// subscribe to both the consensus set and the transaction pool.
type TransactionPoolSubscriber interface {
	// ReceiveTransactionPoolUpdate notifies subscribers of a change to the
	// consensus set and/or unconfirmed set.
	ReceiveTransactionPoolUpdate(revertedBlocks, appliedBlocks []consensus.Block, revertedTxns, appliedTxns []consensus.Transaction)
}

type TransactionPool interface {
	// AcceptTransaction takes a transaction, analyzes it, and either rejects
	// it or adds it to the transaction pool, returning an error if the
	// transaction is rejected.
	AcceptTransaction(consensus.Transaction) error

	// FullTransactionSet returns all of the transactions in the transaction
	// pool, ordered such that any dependencies always come after their
	// requirements. The list of transactions returned may not fit inside of a
	// single block.
	FullTransactionSet() []consensus.Transaction

	// IsStandardTransaction returns `err = nil` if the transaction is
	// standard, otherwise it returns an error explaining what is not standard.
	IsStandardTransaction(consensus.Transaction) error

	// TransactionSet will return a set of transactions not exceeding the block
	// size that can be inserted into a block in order.
	TransactionSet() []consensus.Transaction

	// TransactionPoolSubscribe will subscribe the input object to the changes
	// in the transaction pool.
	TransactionPoolSubscribe(TransactionPoolSubscriber)

	// OutputDiffs returns the set of diffs that are in the transaction pool
	// but haven't been confirmed by a block yet.
	UnconfirmedSiacoinOutputDiffs() []consensus.SiacoinOutputDiff
}
