package modules

import (
	"errors"

	"github.com/NebulousLabs/Sia/types"
)

const (
	TransactionSizeLimit = 16 * 1024
)

var (
	// ErrTransactionPoolDuplicate is returned when a duplicate transaction is
	// submitted to the transaction pool.
	ErrTransactionPoolDuplicate = errors.New("transaction is a duplicate")
	ErrInvalidArbPrefix         = errors.New("transaction contains non-standard arbitrary data")

	PrefixNonSia    = types.Specifier{'N', 'o', 'n', 'S', 'i', 'a'}
	PrefixStrNonSia = "NonSia" // COMPATv0.3.3.3
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
	AcceptTransaction(types.Transaction) error

	// RelayTransaction is an RPC that accepts a block from a peer.
	RelayTransaction(PeerConn) error

	// IsStandardTransaction returns `err = nil` if the transaction is
	// standard, otherwise it returns an error explaining what is not standard.
	IsStandardTransaction(types.Transaction) error

	// PurgeTransactionPool is a temporary function available to the miner. In
	// the event that a miner mines an unacceptable block, the transaction pool
	// will be purged to clear out the transaction pool and get rid of the
	// illegal transaction. This should never happen, however there are bugs
	// that make this condition necessary.
	PurgeTransactionPool()

	// TransactionSet returns the set of unconfirmed transactions.
	TransactionSet() []types.Transaction

	// TransactionPoolNotify will push a struct down the channel any time that
	// the transaction pool updates. An update occurs any time there is a new
	// transaction or block introduced to the transaction pool.
	TransactionPoolNotify() <-chan struct{}

	// TransactionPoolSubscribe will subscribe the input object to the changes
	// in the transaction pool.
	TransactionPoolSubscribe(TransactionPoolSubscriber)
}
