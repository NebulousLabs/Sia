package transactionpool

import (
	"sort"

	"github.com/NebulousLabs/Sia/encoding"
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
	tp.mu.Lock()

	// Update the database of confirmed transactions.
	for _, block := range cc.RevertedBlocks {
		if tp.blockHeight > 0 || block.ID() != types.GenesisID {
			tp.blockHeight--
		}
		for _, txn := range block.Transactions {
			err := tp.deleteTransaction(tp.dbTx, txn.ID())
			if err != nil {
				tp.log.Println("ERROR: could not delete a transaction:", err)
			}
		}

		// Pull the transactions out of the fee summary. For estimating only
		// over 10 blocks, it is extermely likely that there will be more
		// applied blocks than reverted blocks, and if there aren't (a height
		// decreasing reorg), there will be more than 10 applied blocks.
		if len(tp.txnsPerBlock) > 0 {
			tailOffset := uint64(0)
			for i := 0; i < len(tp.txnsPerBlock)-1; i++ {
				tailOffset += tp.txnsPerBlock[i]
			}

			// Strip out all of the transactions in this block.
			tp.recentConfirmedFees = tp.recentConfirmedFees[:tailOffset]
			// Strip off the tail offset.
			tp.txnsPerBlock = tp.txnsPerBlock[:len(tp.txnsPerBlock)-1]
		}
	}
	for _, block := range cc.AppliedBlocks {
		if tp.blockHeight > 0 || block.ID() != types.GenesisID {
			tp.blockHeight++
		}
		for _, txn := range block.Transactions {
			err := tp.putTransaction(tp.dbTx, txn.ID())
			if err != nil {
				tp.log.Println("ERROR: could not add a transaction:", err)
			}
		}

		// Add the transactions from this block.
		var totalSize uint64
		for _, txn := range block.Transactions {
			var feeSum types.Currency
			size := uint64(len(encoding.Marshal(txn)))
			for _, fee := range txn.MinerFees {
				feeSum = feeSum.Add(fee)
			}
			feeAvg := feeSum.Div64(size)
			tp.recentConfirmedFees = append(tp.recentConfirmedFees, feeSummary{
				Fee:  feeAvg,
				Size: size,
			})
			totalSize += size
		}
		// Add an extra zero-fee tranasction for any unused block space.
		remaining := types.BlockSizeLimit - totalSize
		tp.recentConfirmedFees = append(tp.recentConfirmedFees, feeSummary{
			Fee:  types.ZeroCurrency,
			Size: remaining, // fine if remaining is zero.
		})
		// Mark the number of fee transactions in this block.
		tp.txnsPerBlock = append(tp.txnsPerBlock, uint64(len(block.Transactions)+1))

		// If there are more than 10 blocks recorded in the txnsPerBlock, strip
		// off the oldest blocks.
		for len(tp.txnsPerBlock) > blockFeeEstimationDepth {
			tp.recentConfirmedFees = tp.recentConfirmedFees[tp.txnsPerBlock[0]:]
			tp.txnsPerBlock = tp.txnsPerBlock[1:]
		}

		// Sort the recent confirmed fees, then scroll forward 10MB and set the
		// median to that txn. First we need to create and copy the fees into a
		// new slice so that the don't get jumbled.
		//
		// TODO: Sort can be altered for quickSelect, which can grab the median
		// in constant time. When counting the number of elements on either side
		// of the pivot, use the 'size' field instead of counting each element
		// as one.
		replica := make([]feeSummary, len(tp.recentConfirmedFees))
		copy(replica, tp.recentConfirmedFees)
		sort.Slice(replica, func(i, j int) bool {
			return replica[i].Fee.Cmp(replica[j].Fee) < 0
		})
		// Scroll through the sorted fees until hitting the median.
		var progress uint64
		for i := range tp.recentConfirmedFees {
			progress += tp.recentConfirmedFees[i].Size
			if progress > (uint64(len(tp.txnsPerBlock))*types.BlockSizeLimit)/2 {
				tp.recentMedianFee = tp.recentConfirmedFees[i].Fee
				break
			}
		}
	}
	err := tp.putRecentConsensusChange(tp.dbTx, cc.ID)
	if err != nil {
		tp.log.Println("ERROR: could not update the recent consensus change:", err)
	}
	err = tp.putBlockHeight(tp.dbTx, tp.blockHeight)
	if err != nil {
		tp.log.Println("ERROR: could not update the block height:", err)
	}
	err = tp.putFeeMedian(tp.dbTx, medianPersist{
		RecentConfirmedFees: tp.recentConfirmedFees,
		TxnsPerBlock:        tp.txnsPerBlock,
		RecentMedianFee:     tp.recentMedianFee,
	})
	if err != nil {
		tp.log.Println("ERROR: could not update the transaction pool median fee information:", err)
	}

	// Scan the applied blocks for transactions that got accepted. This will
	// help to determine which transactions to remove from the transaction
	// pool. Having this list enables both efficiency improvements and helps to
	// clean out transactions with no dependencies, such as arbitrary data
	// transactions from the host.
	txids := make(map[types.TransactionID]struct{})
	for _, block := range cc.AppliedBlocks {
		for _, txn := range block.Transactions {
			txids[txn.ID()] = struct{}{}
		}
	}

	// Save all of the current unconfirmed transaction sets into a list.
	var unconfirmedSets [][]types.Transaction
	for _, tSet := range tp.transactionSets {
		// Compile a new transaction set the removes all transactions duplicated
		// in the block. Though mostly handled by the dependency manager in the
		// transaction pool, this should both improve efficiency and will strip
		// out duplicate transactions with no dependencies (arbitrary data only
		// transactions)
		var newTSet []types.Transaction
		for _, txn := range tSet {
			_, exists := txids[txn.ID()]
			if !exists {
				newTSet = append(newTSet, txn)
			}
		}
		unconfirmedSets = append(unconfirmedSets, newTSet)
	}

	// Purge the transaction pool. Some of the transactions sets may be invalid
	// after the consensus change.
	tp.purge()

	// prune transactions older than maxTxnAge.
	for i, tSet := range unconfirmedSets {
		var validTxns []types.Transaction
		for _, txn := range tSet {
			seenHeight, seen := tp.transactionHeights[txn.ID()]
			if tp.blockHeight-seenHeight <= maxTxnAge || !seen {
				validTxns = append(validTxns, txn)
			} else {
				delete(tp.transactionHeights, txn.ID())
			}
		}
		unconfirmedSets[i] = validTxns
	}

	// Scan through the reverted blocks and re-add any transactions that got
	// reverted to the tpool.
	for i := len(cc.RevertedBlocks) - 1; i >= 0; i-- {
		block := cc.RevertedBlocks[i]
		for _, txn := range block.Transactions {
			// Check whether this transaction has already be re-added to the
			// consensus set by the applied blocks.
			_, exists := txids[txn.ID()]
			if exists {
				continue
			}

			// Try adding the transaction back into the transaction pool.
			tp.acceptTransactionSet([]types.Transaction{txn}, cc.TryTransactionSet) // Error is ignored.
		}
	}

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
		for _, txn := range set {
			err := tp.acceptTransactionSet([]types.Transaction{txn}, cc.TryTransactionSet)
			if err != nil {
				// The transaction is no longer valid, delete it from the
				// heights map to prevent a memory leak.
				delete(tp.transactionHeights, txn.ID())
			}
		}
	}

	// Inform subscribers that an update has executed.
	tp.mu.Demote()
	tp.updateSubscribersTransactions()
	tp.mu.DemotedUnlock()
}

// PurgeTransactionPool deletes all transactions from the transaction pool.
func (tp *TransactionPool) PurgeTransactionPool() {
	tp.mu.Lock()
	tp.purge()
	tp.mu.Unlock()
}
