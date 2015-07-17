package transactionpool

import (
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	TransactionPoolSizeLimit  = 5e6
	TransactionPoolSizeForFee = 1e6
)

var (
	ErrObjectConflict      = errors.New("transaction set conflicts with an existing transaction set")
	ErrFullTransactionPool = errors.New("transaction pool cannot accept more transactions")
	ErrLowMinerFees        = errors.New("transaction set needs more miner fees to be accepted")

	TransactionMinFee = types.NewCurrency64(2).Mul(types.SiacoinPrecision)
)

// checkMinerFees checks that the total amount of transaction fees in the set
// is sufficient to store it in the unconfirmed transactions database.
func (tp *TransactionPool) checkMinerFees(ts []types.Transaction) error {
	if tp.databaseSize > TransactionPoolSizeLimit {
		return ErrFullTransactionPool
	}

	if tp.databaseSize > TransactionPoolSizeForFee {
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
		return modules.ErrTransactionPoolDuplicate
	}

	// Check that the transaction set has enough fees to justify adding it to
	// the database.
	err := tp.checkMinerFees(ts)
	if err != nil {
		return err
	}

	// All checks after this are expensive.
	//
	// TODO: The transactions are encoded multiple times, when only once is
	// needed (IsStandard + size counting).
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

func (tp *TransactionPool) acceptTransactionSet(ts []types.Transaction) error {
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
		tp.knownObjects[ObjectID(diff.ID)] = setID
	}
	for _, diff := range cc.FileContractDiffs {
		tp.knownObjects[ObjectID(diff.ID)] = setID
	}
	for _, diff := range cc.SiafundOutputDiffs {
		tp.knownObjects[ObjectID(diff.ID)] = setID
	}
	tp.transactionSetDiffs[setID] = cc
	tp.databaseSize += len(encoding.Marshal(ts))
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
	tp.updateSubscribersTransactions()
	go tp.gateway.Broadcast("RelayTransactionSet", ts)
	return nil
}

// RelayTransaction is an RPC that accepts a transaction from a peer. If the
// accept is successful, the transaction will be relayed to the Gateway's
// other peers.
func (tp *TransactionPool) RelayTransactionSet(conn modules.PeerConn) error {
	var ts []types.Transaction
	err := encoding.ReadObject(conn, &ts, types.BlockSizeLimit)
	if err != nil {
		return err
	}
	err = tp.AcceptTransactionSet(ts)
	if err == modules.ErrTransactionPoolDuplicate { // benign error
		err = nil
	}
	return err
}

// DEPRECATED
func (tp *TransactionPool) AcceptTransaction(t types.Transaction) error {
	return tp.AcceptTransactionSet([]types.Transaction{t})
}

// DEPRECATED
func (tp *TransactionPool) RelayTransaction(conn modules.PeerConn) error {
	return tp.RelayTransactionSet(conn)
}
