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
	tp.databaseSize = 0
}

// ReceiveConsensusSetUpdate gets called to inform the transaction pool of
// changes to the consensus set.
func (tp *TransactionPool) ReceiveConsensusSetUpdate(cc modules.ConsensusChange) {
	lockID := tp.mu.Lock()
	defer tp.mu.Unlock(lockID)

	// Save all of the reverted transactions. Transactions need to appear in
	// 'unconfirmedTxns' in the same order that they would appear in the
	// blockchain. 'revertedBlocks' is backwards (first element has highest
	// height), so each time a new block processed, the transactions need to be
	// prepended to the list of unconfirmed transactions.
	var revertedTxns []types.Transaction
	for _, block := range cc.RevertedBlocks {
		revertedTxns = append(block.Transactions, revertedTxns...)
	}

	// Add all of the current unconfirmed transactions to the unconfirmed
	// transaction list.
	unconfirmedSets := [][]types.Transaction{revertedTxns}
	for _, tSet := range tp.transactionSets {
		unconfirmedSets = append(unconfirmedSets, tSet)
	}

	// Purge the pool of unconfirmed transactions so that there is no
	// interference from unconfirmed transactions during the application of
	// potentially conflicting transactions that have been added to the
	// blockchain.
	tp.purge()

	// Add all potential unconfirmed transactions back into the pool after
	// checking that they are still valid.
	for _, set := range unconfirmedSets {
		// Error does not need to be checked - some will fail because the block
		// height has changed and new transactions have been confirmed.
		_ = tp.acceptTransactionSet(set)
	}

	// Inform subscribers that an update has executed.
	println("incrementational")
	tp.consensusChangeIndex++
	tp.updateSubscribersConsensus(cc)
	tp.updateSubscribersTransactions()
}

// PurgeTransactionPool deletes all transactions from the transaction pool.
// It's a failsafe for when the transaction pool is producing invalid
// transaction sets.
func (tp *TransactionPool) PurgeTransactionPool() {
	lockID := tp.mu.Lock()
	tp.purge()
	tp.mu.Unlock(lockID)
}
