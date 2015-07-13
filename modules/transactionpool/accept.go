package transactionpool

import (
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	TransactionPoolSizeLimit  = 10 * 1024 * 1024
	TransactionPoolSizeForFee = 5 * 1024 * 1024
)

var (
	ErrLargeTransactionPool = errors.New("transaction size limit reached within pool")
	ErrLowMinerFees         = errors.New("transaction miner fees too low to be accepted")
	TransactionMinFee       = types.NewCurrency64(3).Mul(types.SiacoinPrecision)
)

// accept.go is responsible for applying a transaction to the transaction pool.
// Validation is handled by valid.go. The componenets of the transcation are
// added to the unconfirmed consensus set piecemeal, and then the transaction
// itself is appended to the linked list of transactions, such that any
// dependecies will appear earlier in the list.

// checkMinerFees checks that all MinerFees are valid within the context of the
// transactionpool given parameters to prevention DoS
func (tp *TransactionPool) checkMinerFees(t types.Transaction) error {
	// TODO: This has unacceptable order notation.
	transactionPoolSize := len(encoding.Marshal(tp.transactionList))
	if transactionPoolSize > TransactionPoolSizeLimit {
		return ErrLargeTransactionPool
	}
	if transactionPoolSize > TransactionPoolSizeForFee {
		var feeSum types.Currency
		for _, fee := range t.MinerFees {
			feeSum = feeSum.Add(fee)
		}
		if feeSum.Cmp(TransactionMinFee) < 0 {
			return ErrLowMinerFees
		}
	}
	return
}

// addTransactionToPool puts a transaction into the transaction pool, changing
// the unconfirmed set and the transaction linked list to reflect the new
// transaction.
func (tp *TransactionPool) addTransactionToPool(t types.Transaction) {
	// Add the transaction to the list of transactions.
	tp.transactions[crypto.HashObject(t)] = struct{}{}
	tp.transactionList = append(tp.transactionList, t)
}

// AcceptTransaction adds a transaction to the unconfirmed set of
// transactions. If the transaction is accepted, it will be relayed to
// connected peers.
func (tp *TransactionPool) AcceptTransactions(ts []types.Transaction) (err error) {
	id := tp.mu.Lock()
	defer tp.mu.Unlock(id)

	// Check that the transaction set is not currently in the unconfirmed set.
	setHash := TransactionSetID(crypto.HashObject(ts))
	_, exists := tp.transactionSets[setHash]
	if exists {
		return modules.ErrTransactionPoolDuplicate
	}

	// Check that the transaction follows 'Standard.md' guidelines.
	err = tp.IsStandardTransaction(t)
	if err != nil {
		return err
	}

	// Check that the transaction has enough fees to justify it being in the
	// mempool.
	err = tp.checkMinerFees(t)
	if err != nil {
		return
	}

	// Check that the transaction is legal given the unconfirmed consensus set
	// and the settings of the transaction pool.
	err = tp.validUnconfirmedTransaction(t)
	if err != nil {
		return
	}

	// Add the transaction to the pool, notify all subscribers, and broadcast
	// the transaction.
	tp.addTransactionToPool(t)
	tp.updateSubscribers(modules.ConsensusChange{}, tp.transactionList, tp.unconfirmedSiacoinOutputDiffs())
	go tp.gateway.Broadcast("RelayTransaction", t)
	return
}

// RelayTransaction is an RPC that accepts a transaction from a peer. If the
// accept is successful, the transaction will be relayed to the Gateway's
// other peers.
func (tp *TransactionPool) RelayTransaction(conn modules.PeerConn) error {
	var t types.Transaction
	err := encoding.ReadObject(conn, &t, types.BlockSizeLimit)
	if err != nil {
		return err
	}
	err = tp.AcceptTransaction(t)
	// ErrTransactionPoolDuplicate is benign
	// TODO: Is relay called automatically? Will it broadcast a dup? If it
	// broadcasts a dup, this is a severe DoS problem.
	if err == modules.ErrTransactionPoolDuplicate {
		err = nil
	}
	return err
}
