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
	errObjectConflict      = errors.New("transaction set conflicts with an existing transaction set")
	errFullTransactionPool = errors.New("transaction pool cannot accept more transactions")
	errLowMinerFees        = errors.New("transaction set needs more miner fees to be accepted")
	errEmptySet            = errors.New("transaction set is empty")

	TransactionMinFee = types.NewCurrency64(2).Mul(types.SiacoinPrecision)
)

// relatedObjectIDs determines all of the object ids related to a transaction.
func relatedObjectIDs(ts []types.Transaction) []ObjectID {
	oidMap := make(map[ObjectID]struct{})
	for _, t := range ts {
		for _, sci := range t.SiacoinInputs {
			oidMap[ObjectID(sci.ParentID)] = struct{}{}
		}
		for i := range t.SiacoinOutputs {
			oidMap[ObjectID(t.SiacoinOutputID(uint64(i)))] = struct{}{}
		}
		for i := range t.FileContracts {
			oidMap[ObjectID(t.FileContractID(uint64(i)))] = struct{}{}
		}
		for _, fcr := range t.FileContractRevisions {
			oidMap[ObjectID(fcr.ParentID)] = struct{}{}
		}
		for _, sp := range t.StorageProofs {
			oidMap[ObjectID(sp.ParentID)] = struct{}{}
		}
		for _, sfi := range t.SiafundInputs {
			oidMap[ObjectID(sfi.ParentID)] = struct{}{}
		}
		for i := range t.SiafundOutputs {
			oidMap[ObjectID(t.SiafundOutputID(uint64(i)))] = struct{}{}
		}
	}

	var oids []ObjectID
	for oid, _ := range oidMap {
		oids = append(oids, oid)
	}
	return oids
}

// checkMinerFees checks that the total amount of transaction fees in the
// transaction set is sufficient to earn a spot in the transaction pool.
func (tp *TransactionPool) checkMinerFees(ts []types.Transaction) error {
	// Transactions cannot be added after the TransactionPoolSizeLimit has been
	// hit.
	if tp.transactionListSize > TransactionPoolSizeLimit {
		return errFullTransactionPool
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
			return errLowMinerFees
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

// handleConflicts detects whether the conflicts in the transaction pool are
// legal children of the new transaction pool set or not.
func (tp *TransactionPool) handleConflicts(ts []types.Transaction, conflicts []TransactionSetID) error {
	// Create a list of all the transaction ids that compose the set of
	// conflicts.
	conflictMap := make(map[types.TransactionID]TransactionSetID)
	for _, conflict := range conflicts {
		conflictSet := tp.transactionSets[conflict]
		for _, conflictTxn := range conflictSet {
			conflictMap[conflictTxn.ID()] = conflict
		}
	}

	// Discard all duplicate transactions from the input transaction set.
	var dedupSet []types.Transaction
	for _, t := range ts {
		_, exists := conflictMap[t.ID()]
		if exists {
			continue
		}
		dedupSet = append(dedupSet, t)
	}
	if len(dedupSet) == 0 {
		return modules.ErrDuplicateTransactionSet
	}
	// If transactions were pruned, it's possible that the set of
	// dependencies/conflicts has also reduced. To minimize computational load
	// on the consensus set, we want to prune out all of the conflicts that are
	// no longer relevant. As an example, consider the transaction set {A}, the
	// set {B}, and the new set {A, C}, where C is dependent on B. {A} and {B}
	// are both conflicts, but after deduplication {A} is no longer a conflict.
	// This is recursive, but it is guaranteed to run only once as the first
	// deduplication is guaranteed to be complete.
	if len(dedupSet) < len(ts) {
		oids := relatedObjectIDs(dedupSet)
		var conflicts []TransactionSetID
		for _, oid := range oids {
			conflict, exists := tp.knownObjects[oid]
			if exists {
				conflicts = append(conflicts, conflict)
			}
		}
		return tp.handleConflicts(dedupSet, conflicts)
	}

	// Merge all of the conflict sets with the input set (input set goes last
	// to preserve dependency ordering), and see if the set as a whole is both
	// small enough to be legal and valid as a set. If no, return an error. If
	// yes, add the new set to the pool, and eliminate the old set. The output
	// diff objects can be repeated, (no need to remove those). Just need to
	// remove the conflicts from tp.transactionSets.
	var superset []types.Transaction
	supersetMap := make(map[TransactionSetID]struct{})
	for _, conflict := range conflictMap {
		supersetMap[conflict] = struct{}{}
	}
	for conflict := range supersetMap {
		superset = append(superset, tp.transactionSets[conflict]...)
	}
	superset = append(superset, dedupSet...)

	// Check the composition of the transaction set, including fees and
	// IsStandard rules (this is a new set, the rules must be rechecked).
	err := tp.checkTransactionSetComposition(superset)
	if err != nil {
		return err
	}

	// Check that the transaction set is valid.
	cc, err := tp.consensusSet.TryTransactionSet(superset)
	if err != nil {
		return modules.NewConsensusConflict(err.Error())
	}

	// Remove the conflicts from the transaction pool. The diffs do not need to
	// be removed, they will be overwritten later in the function.
	for _, conflict := range conflictMap {
		conflictSet := tp.transactionSets[conflict]
		tp.transactionListSize -= len(encoding.Marshal(conflictSet))
		delete(tp.transactionSets, conflict)
		delete(tp.transactionSetDiffs, conflict)
	}

	// Add the transaction set to the pool.
	setID := TransactionSetID(crypto.HashObject(superset))
	tp.transactionSets[setID] = superset
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
	tp.transactionListSize += len(encoding.Marshal(superset))
	return nil
}

// acceptTransactionSet verifies that a transaction set is allowed to be in the
// transaction pool, and then adds it to the transaction pool.
func (tp *TransactionPool) acceptTransactionSet(ts []types.Transaction) error {
	if len(ts) == 0 {
		return errEmptySet
	}

	// Check the composition of the transaction set, including fees and
	// IsStandard rules.
	err := tp.checkTransactionSetComposition(ts)
	if err != nil {
		return err
	}

	// Check for conflicts with other transactions, which would indicate a
	// double-spend. Legal children of a transaction set will also trigger the
	// conflict-detector.
	oids := relatedObjectIDs(ts)
	var conflicts []TransactionSetID
	for _, oid := range oids {
		conflict, exists := tp.knownObjects[oid]
		if exists {
			conflicts = append(conflicts, conflict)
		}
	}
	if len(conflicts) > 0 {
		return tp.handleConflicts(ts, conflicts)
	}
	cc, err := tp.consensusSet.TryTransactionSet(ts)
	if err != nil {
		return modules.NewConsensusConflict(err.Error())
	}

	// Add the transaction set to the pool.
	setID := TransactionSetID(crypto.HashObject(ts))
	tp.transactionSets[setID] = ts
	for _, oid := range oids {
		tp.knownObjects[oid] = setID
	}
	tp.transactionSetDiffs[setID] = cc
	tp.transactionListSize += len(encoding.Marshal(ts))
	return nil
}

// AcceptTransaction adds a transaction to the unconfirmed set of
// transactions. If the transaction is accepted, it will be relayed to
// connected peers.
func (tp *TransactionPool) AcceptTransactionSet(ts []types.Transaction) error {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	err := tp.acceptTransactionSet(ts)
	if err != nil {
		return err
	}

	// Notify subscribers and broadcast the transaction set.
	go tp.gateway.Broadcast("RelayTransactionSet", ts)
	tp.updateSubscribersTransactions()

	return nil
}

// relayTransactionSet is an RPC that accepts a transaction set from a peer. If
// the accept is successful, the transaction will be relayed to the gateway's
// other peers.
func (tp *TransactionPool) relayTransactionSet(conn modules.PeerConn) error {
	var ts []types.Transaction
	err := encoding.ReadObject(conn, &ts, types.BlockSizeLimit)
	if err != nil {
		return err
	}
	return tp.AcceptTransactionSet(ts)
}
