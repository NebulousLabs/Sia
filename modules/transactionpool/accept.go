package transactionpool

import (
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// The TransactionPoolSizeLimit is first checked, and then a transaction
	// set is added. The current transaction pool does not do any priority
	// ordering, so the size limit is such that the transaction pool will never
	// exceed the size of a block.
	//
	// TODO: Add a priority structure that will allow the transaction pool to
	// fill up beyond the size of a single block, without being subject to
	// manipulation.
	//
	// The first ~1/4 of the transaction pool can be filled for free. This is
	// mostly to preserve compatibility with clients that do not add fees.
	TransactionPoolSizeLimit  = 2e6 - 5e3 - modules.TransactionSetSizeLimit
	TransactionPoolSizeForFee = 500e3
)

var (
	ErrObjectConflict      = errors.New("transaction set conflicts with an existing transaction set")
	ErrFullTransactionPool = errors.New("transaction pool cannot accept more transactions")
	ErrLowMinerFees        = errors.New("transaction set needs more miner fees to be accepted")

	TransactionMinFee = types.NewCurrency64(2).Mul(types.SiacoinPrecision)
)

// checkMinerFees checks that the total amount of transaction fees in the
// transaction set is sufficient to earn a spot in the transaction pool.
func (tp *TransactionPool) checkMinerFees(ts []types.Transaction) error {
	// Transactions cannot be added after the TransactionPoolSizeLimit has been
	// hit.
	if tp.transactionListSize > TransactionPoolSizeLimit {
		return ErrFullTransactionPool
	}

	// The first TransactionPoolSizeForFee transactions do not need fees.
	if tp.transactionListSize > TransactionPoolSizeForFee {
		// Currently required fees are set on a per-transaction basis. 2 coins
		// are required per transaction if the free-fee limit has been reached,
		// adding a larger fee is not useful.
		var feeSum types.Currency
		for i := range ts {
			for _, fee := range ts[i].MinerFees {
				feeSum = feeSum.Add(fee)
			}
		}
		feeRequired := TransactionMinFee.Mul(types.NewCurrency64(uint64(len(ts))))
		if feeSum.Cmp(feeRequired) < 0 {
			return ErrLowMinerFees
		}
	}
	return nil
}

// checkTransactionSetComposition checks if the transaction set is valid given
// the state of the pool. It does not check that each individual transaction
// would be legal in the next block, but does check things like miner fees and
// IsStandard.
func (tp *TransactionPool) checkTransactionSetComposition(ts []types.Transaction) error {
	// Check that the transaction set is not already known.
	setID := TransactionSetID(crypto.HashObject(ts))
	_, exists := tp.transactionSets[setID]
	if exists {
		return modules.ErrDuplicateTransactionSet
	}

	// Check that the transaction set has enough fees to justify adding it to
	// the transaction list.
	err := tp.checkMinerFees(ts)
	if err != nil {
		return err
	}

	// All checks after this are expensive.
	//
	// TODO: There is no DoS prevention mechanism in place to prevent repeated
	// expensive verifications of invalid transactions that are created on the
	// fly.

	// Check that all transactions follow 'Standard.md' guidelines.
	err = tp.IsStandardTransactionSet(ts)
	if err != nil {
		return err
	}
	return nil
}

// acceptTransactionSet verifies that a transaction set is allowed to be in the
// transaction pool, and then adds it to the transaction pool.
func (tp *TransactionPool) acceptTransactionSet(ts []types.Transaction) error {
	// Check the composition of the transaction set, including fees and
	// IsStandard rules.
	err := tp.checkTransactionSetComposition(ts)
	if err != nil {
		return err
	}

	// Check that all transactions are valid, and that there is no conflict
	// with existing transactions.
	cc, err := tp.consensusSet.TryTransactionSet(ts)
	if err != nil {
		return err
	}
	for _, diff := range cc.SiacoinOutputDiffs {
		_, exists := tp.knownObjects[ObjectID(diff.ID)]
		if exists {
			return ErrObjectConflict
		}
	}
	for _, diff := range cc.FileContractDiffs {
		_, exists := tp.knownObjects[ObjectID(diff.ID)]
		if exists {
			return ErrObjectConflict
		}
	}
	for _, diff := range cc.SiafundOutputDiffs {
		_, exists := tp.knownObjects[ObjectID(diff.ID)]
		if exists {
			return ErrObjectConflict
		}
	}

	// Add the transaction set to the pool.
	setID := TransactionSetID(crypto.HashObject(ts))
	tp.transactionSets[setID] = ts
	for _, diff := range cc.SiacoinOutputDiffs {
		tp.knownObjects[ObjectID(diff.ID)] = struct{}{}
	}
	for _, diff := range cc.FileContractDiffs {
		tp.knownObjects[ObjectID(diff.ID)] = struct{}{}
	}
	for _, diff := range cc.SiafundOutputDiffs {
		tp.knownObjects[ObjectID(diff.ID)] = struct{}{}
	}
	tp.transactionSetDiffs[setID] = cc
	tp.transactionListSize += len(encoding.Marshal(ts))
	return nil
}

// AcceptTransaction adds a transaction to the unconfirmed set of
// transactions. If the transaction is accepted, it will be relayed to
// connected peers.
func (tp *TransactionPool) AcceptTransactionSet(ts []types.Transaction) error {
	id := tp.mu.Lock()
	defer tp.mu.Unlock(id)

	err := tp.acceptTransactionSet(ts)
	if err != nil {
		return err
	}

	// Notify subscribers and broadcast the transaction set.
	go tp.gateway.Broadcast("RelayTransactionSet", ts)
	tp.updateSubscribersTransactions()
	return nil
}

// RelayTransaction is an RPC that accepts a transaction set from a peer. If
// the accept is successful, the transaction will be relayed to the gateway's
// other peers.
func (tp *TransactionPool) RelayTransactionSet(conn modules.PeerConn) error {
	var ts []types.Transaction
	err := encoding.ReadObject(conn, &ts, types.BlockSizeLimit)
	if err != nil {
		return err
	}
	// TODO: Ask Luke some stuff about DoS with regards to this function.
	err = tp.AcceptTransactionSet(ts)
	if err == modules.ErrDuplicateTransactionSet { // benign error
		err = nil
	}
	return err
}
