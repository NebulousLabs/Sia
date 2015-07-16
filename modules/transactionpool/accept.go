package transactionpool

import (
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	TransactionPoolSizeLimit  = 10e6
	TransactionPoolSizeForFee = 2e6
)

var (
	ErrObjectConflict = errors.New("transaction set conflicts with an existing transaction set")
	ErrFullTransactionPool = errors.New("transaction pool cannot accept more transactions")
	ErrLowMinerFees         = errors.New("transaction set needs more miner fees to be accepted")
	TransactionMinFee       = types.NewCurrency64(2).Mul(types.SiacoinPrecision)
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
	return
}

// AcceptTransaction adds a transaction to the unconfirmed set of
// transactions. If the transaction is accepted, it will be relayed to
// connected peers.
func (tp *TransactionPool) AcceptTransactions(ts []types.Transaction) (err error) {
	id := tp.mu.Lock()
	defer tp.mu.Unlock(id)

	// Check that the transaction set is not already known.
	setHash := TransactionSetID(crypto.HashObject(ts))
	_, exists := tp.transactionSets[setHash]
	if exists {
		return modules.ErrTransactionPoolDuplicate
	}

	// Check that the transaction set has enough fees to justify adding it to
	// the database.
	err = tp.checkMinerFees(ts)
	if err != nil {
		return
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

	// Check that all transactions are valid, and that there is no conflict
	// with existing transacitons.
	cc, err := tp.consensusSet.TryTransactions(ts)
	if err != nil {
		return err
	}
	for _, diff := range cc.SiacoinOutputDiffs {
		_, exists := tp.knownObjects[crypto.Hash(diff.ID)]
		if exists {
			return ErrObjectConflict
		}
	}
	for _, diff := range cc.FileContractDiffs {
		_, exists := tp.knownObjects[crypto.Hash(diff.ID)]
		if exists {
			return ErrObjectConflict
		}
	}
	for _, diff := range cc.SiafundOutputDiffs {
		_, exists := tp.knownObjects[crypto.Hash(diff.ID)]
		if exists {
			return ErrObjectConflict
		}
	}

	// Add the transaction to the pool.
	tp.transactionSets[setHash] = ts
	var oids []ObjectID
	for _, diff := range cc.SiacoinOutputDiffs {
		tp.knownObjects = crypto.Hash(diff.ID)
		oids = append(oids, crypto.Hash(diff.ID))
	}
	for _, diff := range cc.FileContractDiffs {
		tp.knownObjects = crypto.Hash(diff.ID)
		oids = append(oids, crypto.Hash(diff.ID))
	}
	for _, diff := range cc.SiafundOutputDiffs {
		tp.knownObjects = crypto.Hash(diff.ID)
		oids = append(oids, crypto.Hash(diff.ID))
	}
	tp.transactionSetDiffs[setHash] = oids
	tp.databaseSize += len(encoding.Marshal(ts))

	// Notify subscribers and broadcast the transaction set.
	tp.updateSubscribers(modules.ConsensusChange{}, tp.transactionList, tp.unconfirmedSiacoinOutputDiffs())
	go tp.gateway.Broadcast("RelayTransactionSet", ts)
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
