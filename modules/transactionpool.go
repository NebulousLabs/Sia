package modules

import (
	"errors"

	"github.com/NebulousLabs/Sia/types"
)

const (
	TransactionSizeLimit    = 16e3
	TransactionSetSizeLimit = 100e3
)

var (
	// ErrTransactionPoolDuplicate is returned when a duplicate transaction is
	// submitted to the transaction pool.
	ErrTransactionPoolDuplicate = errors.New("transaction is a duplicate")
	ErrInvalidArbPrefix         = errors.New("transaction contains non-standard arbitrary data")

	PrefixNonSia    = types.Specifier{'N', 'o', 'n', 'S', 'i', 'a'}
	PrefixStrNonSia = "NonSia" // COMPATv0.3.3.3

	TransactionPoolDir = "transactionpool"
)

// A TransactionPoolSubscriber receives updates about the confirmed and
// unconfirmed set from the transaction pool. Generally, there is no need to
// subscribe to both the consensus set and the transaction pool.
type TransactionPoolSubscriber interface {
	// All transaction pool subscribers must also be able to receive consensus
	// set updates.
	ConsensusSetSubscriber

	// ReceiveTransactionPoolUpdate notifies subscribers of a change to the
	// consensus set and/or unconfirmed set, and includes the consensus change
	// that would result if all of the transactions made it into a block.
	ReceiveUpdatedUnconfirmedTransactions([]types.Transaction, ConsensusChange)
}

type TransactionPool interface {
	// AcceptTransaction takes a transaction, analyzes it, and either rejects
	// it or adds it to the transaction pool. Accepted transactions will be
	// relayed to connected peers.
	//
	// DEPRECATED
	AcceptTransaction(types.Transaction) error

	AcceptTransactionSet([]types.Transaction) error

	// RelayTransactionSet is an RPC that accepts a transaction set from a
	// peer.
	RelayTransactionSet(PeerConn) error

	// IsStandardTransaction returns `err = nil` if the transaction is
	// standard, otherwise it returns an error explaining what is not standard.
	IsStandardTransaction(types.Transaction) error

	// PurgeTransactionPool is a temporary function available to the miner. In
	// the event that a miner mines an unacceptable block, the transaction pool
	// will be purged to clear out the transaction pool and get rid of the
	// illegal transaction. This should never happen, however there are bugs
	// that make this condition necessary.
	PurgeTransactionPool()

	// TransactionList returns a list of all transactions in the transaction
	// pool. The transactions are provided in an order that can acceptably be
	// put into a block.
	TransactionList() []types.Transaction

	// TransactionPoolSubscribe adds a subscriber to the transaction pool.
	// Subscribers will receive all consensus set changes as well as
	// transaction pool changes, and should not subscribe to both.
	TransactionPoolSubscribe(TransactionPoolSubscriber)
}
