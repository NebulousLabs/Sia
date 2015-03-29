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
	ReceiveTransactionPoolUpdate(revertedBlocks, appliedBlocks []consensus.Block, unconfirmedTransactions []consensus.Transaction, unconfirmedSiacoinOutputDiffs []consensus.SiacoinOutputDiff)
}

type TransactionPool interface {
	// AcceptTransaction takes a transaction, analyzes it, and either rejects
	// it or adds it to the transaction pool, returning an error if the
	// transaction is rejected.
	AcceptTransaction(consensus.Transaction) error

	// IsStandardTransaction returns `err = nil` if the transaction is
	// standard, otherwise it returns an error explaining what is not standard.
	IsStandardTransaction(consensus.Transaction) error

	// TransactionSet returns the set of unconfirmed transactions.
	TransactionSet() []consensus.Transaction

	// TransactionPoolSubscribe will subscribe the input object to the changes
	// in the transaction pool.
	TransactionPoolSubscribe(TransactionPoolSubscriber)
}
