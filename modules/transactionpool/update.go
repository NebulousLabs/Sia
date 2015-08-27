package transactionpool

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// purge removes all transactions from the transaction pool.
func (tp *TransactionPool) purge() {
	tp.knownObjects = make(map[ObjectID]TransactionSetID)
	tp.transactionSets = make(map[TransactionSetID][]types.Transaction)
	tp.transactionSetDiffs = make(map[TransactionSetID]modules.ConsensusChange)
	tp.transactionListSize = 0
}

// ProcessConsensusChange gets called to inform the transaction pool of changes
// to the consensus set.
func (tp *TransactionPool) ProcessConsensusChange(cc modules.ConsensusChange) {
	lockID := tp.mu.Lock()

	// TODO: Right now, transactions that were reverted to not get saved and
	// retried, because some transactions such as storage proofs might be
	// illegal, and there's no good way to preserve dependencies when illegal
	// transactions are suddenly involved.
	//
	// One potential solution is to have modules manually do resubmission if
	// something goes wrong. Another is to have the transaction pool remember
	// recent transaction sets on the off chance that they become valid again
	// due to a reorg.
	//
	// Another option is to scan through the blocks transactions one at a time
	// check if they are valid. If so, lump them in a set with the next guy.
	// When they stop being valid, you've found a guy to throw away. It's n^2
	// in the number of transactions in the block.

	// Save all of the current unconfirmed transaction sets into a list.
	var unconfirmedSets [][]types.Transaction
	for _, tSet := range tp.transactionSets {
		unconfirmedSets = append(unconfirmedSets, tSet)
	}

	// Purge the transaction pool. Some of the transactions sets may be invalid
	// after the consensus change.
	tp.purge()

	// Add all of the unconfirmed transaction sets back to the transaction
	// pool. The ones that are invalid will throw an error and will not be
	// re-added.
	//
	// Accepting a transaction set requires locking the consensus set (to check
	// validity). But, ProcessConsensusChange is only called when the consensus
	// set is already locked, causing a deadlock problem. Therefore,
	// transactions are readded to the pool in a goroutine, so that this
	// function can finish and consensus can unlock. The tpool lock is held
	// however until the goroutine completes.
	//
	// Which means that no other modules can require a tpool lock when
	// processing consensus changes. Overall, the locking is pretty fragile and
	// more rules need to be put in place.
	for _, set := range unconfirmedSets {
		tp.acceptTransactionSet(set) // Error is not checked.
	}

	// Inform subscribers that an update has executed.
	tp.consensusChangeIndex++
	tp.updateSubscribersConsensus(cc)
	tp.updateSubscribersTransactions()
	tp.mu.Unlock(lockID)
}

// PurgeTransactionPool deletes all transactions from the transaction pool.
func (tp *TransactionPool) PurgeTransactionPool() {
	lockID := tp.mu.Lock()
	tp.purge()
	tp.mu.Unlock(lockID)
}
