package transactionpool

import (
	"bytes"
	"sort"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// findSets takes a bunch of transactions (presumably from a block) and finds
// all of the separate transaction sets within it. Set does not check for
// conflicts.
//
// The algorithm goes through one transaction at a time. All of the outputs of
// that transaction are added to the objMap, pointing to the transaction to
// indicate that the transaction contains those outputs. The transaction is
// assigned an integer id (each transaction will have a unique id) and added to
// the txMap.
//
// The transaction's inputs are then checked against the objMap to see if there
// are any parents of the transaction in the graph. If there are, the
// transaction is added to the parent set instead of its own set. If not, the
// transaction is added as its own set.
//
// The forwards map contains a list of ints indicating when a transaction has
// been merged with a set. When a transaction gets merged with a parent set, its
// integer id gets added to the forwards map, indicating that the transaction is
// no longer in its own set, but instead has been merged with other sets.
//
// Some transactions will have parents from multiple distinct sets. If a
// transaction has parents in multiple distinct sets, those sets get merged
// together and the transaction gets added to the result. One of the sets is
// nominated (arbitrarily) as the official set, and the integer id of the other
// set and the new transaction get forwarded to the official set.
//
// TODO: Set merging currently occurs any time that there is a child. But
// really, it should only occur if the child increases the average fee value of
// the set that it is merging with (which it will if and only if it has a higher
// average fee than that set). If the child has multiple parent sets, it should
// be compared with the parent set that has the lowest fee value. Then, after it
// is merged with that parent, the result should be merged with the next
// lowest-fee parent set if and only if the new set has a higher average fee
// than the parent set. And this continues until either all of the sets have
// been merged, or until the remaining parent sets have higher values.
func findSets(ts []types.Transaction) [][]types.Transaction {
	// txMap marks what set each transaction is in. If two sets get combined,
	// this number will not be updated. The 'forwards' map defined further on
	// will help to discover which sets have been combined.
	txMap := make(map[types.TransactionID]int)
	setMap := make(map[int][]types.Transaction)
	objMap := make(map[ObjectID]types.TransactionID)
	forwards := make(map[int]int)

	// Define a function to follow and collapse any update chain.
	forward := func(prev int) (ret int) {
		ret = prev
		next, exists := forwards[prev]
		for exists {
			ret = next
			forwards[prev] = next // collapse the forwards function to prevent quadratic runtime of findSets.
			next, exists = forwards[next]
		}
		return ret
	}

	// Add the transactions to the setup one-by-one, merging them as they belong
	// to a set.
	for i, t := range ts {
		// Check if the inputs depend on any previous transaction outputs.
		tid := t.ID()
		parentSets := make(map[int]struct{})
		for _, obj := range t.SiacoinInputs {
			txid, exists := objMap[ObjectID(obj.ParentID)]
			if exists {
				parentSet := forward(txMap[txid])
				parentSets[parentSet] = struct{}{}
			}
		}
		for _, obj := range t.FileContractRevisions {
			txid, exists := objMap[ObjectID(obj.ParentID)]
			if exists {
				parentSet := forward(txMap[txid])
				parentSets[parentSet] = struct{}{}
			}
		}
		for _, obj := range t.StorageProofs {
			txid, exists := objMap[ObjectID(obj.ParentID)]
			if exists {
				parentSet := forward(txMap[txid])
				parentSets[parentSet] = struct{}{}
			}
		}
		for _, obj := range t.SiafundInputs {
			txid, exists := objMap[ObjectID(obj.ParentID)]
			if exists {
				parentSet := forward(txMap[txid])
				parentSets[parentSet] = struct{}{}
			}
		}

		// Determine the new counter for this transaction.
		if len(parentSets) == 0 {
			// No parent sets. Make a new set for this transaction.
			txMap[tid] = i
			setMap[i] = []types.Transaction{t}
			// Don't need to add anything for the file contract outputs, storage
			// proof outputs, siafund claim outputs; these outputs are not
			// allowed to be spent until 50 confirmations.
		} else {
			// There are parent sets, pick one as the base and then merge the
			// rest into it.
			parentsSlice := make([]int, 0, len(parentSets))
			for j := range parentSets {
				parentsSlice = append(parentsSlice, j)
			}
			base := parentsSlice[0]
			txMap[tid] = base
			for _, j := range parentsSlice[1:] {
				// Forward any future transactions pointing at this set to the
				// base set.
				forwards[j] = base
				// Combine the transactions in this set with the transactions in
				// the base set.
				setMap[base] = append(setMap[base], setMap[j]...)
				// Delete this set map, it has been merged with the base set.
				delete(setMap, j)
			}
			// Add this transaction to the base set.
			setMap[base] = append(setMap[base], t)
		}

		// Mark this transaction's outputs as potential inputs to future
		// transactions.
		for j := range t.SiacoinOutputs {
			scoid := t.SiacoinOutputID(uint64(j))
			objMap[ObjectID(scoid)] = tid
		}
		for j := range t.FileContracts {
			fcid := t.FileContractID(uint64(j))
			objMap[ObjectID(fcid)] = tid
		}
		for j := range t.FileContractRevisions {
			fcid := t.FileContractRevisions[j].ParentID
			objMap[ObjectID(fcid)] = tid
		}
		for j := range t.SiafundOutputs {
			sfoid := t.SiafundOutputID(uint64(j))
			objMap[ObjectID(sfoid)] = tid
		}
	}

	// Compile the final group of sets.
	ret := make([][]types.Transaction, 0, len(setMap))
	for _, set := range setMap {
		ret = append(ret, set)
	}
	return ret
}

// purge removes all transactions from the transaction pool.
func (tp *TransactionPool) purge() {
	tp.knownObjects = make(map[ObjectID]TransactionSetID)
	tp.transactionSets = make(map[TransactionSetID][]types.Transaction)
	tp.transactionSetDiffs = make(map[TransactionSetID]*modules.ConsensusChange)
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
		if len(tp.recentMedians) > 0 {
			// Strip out all of the transactions in this block.
			tp.recentMedians = tp.recentMedians[:len(tp.recentMedians)-1]
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

		// Find the median transaction fee for this block.
		type feeSummary struct {
			fee  types.Currency
			size int
		}
		var fees []feeSummary
		var totalSize int
		txnSets := findSets(block.Transactions)
		for _, set := range txnSets {
			// Compile the fees for this set.
			var feeSum types.Currency
			var sizeSum int
			b := new(bytes.Buffer)
			for _, txn := range set {
				txn.MarshalSia(b)
				sizeSum += b.Len()
				b.Reset()
				for _, fee := range txn.MinerFees {
					feeSum = feeSum.Add(fee)
				}
			}
			feeAvg := feeSum.Div64(uint64(sizeSum))
			fees = append(fees, feeSummary{
				fee:  feeAvg,
				size: sizeSum,
			})
			totalSize += sizeSum
		}
		// Add an extra zero-fee tranasction for any unused block space.
		remaining := int(types.BlockSizeLimit) - totalSize
		fees = append(fees, feeSummary{
			fee:  types.ZeroCurrency,
			size: remaining, // fine if remaining is zero.
		})
		// Sort the fees by value and then scroll until the median.
		sort.Slice(fees, func(i, j int) bool {
			return fees[i].fee.Cmp(fees[j].fee) < 0
		})
		var progress int
		for i := range fees {
			progress += fees[i].size
			// Instead of grabbing the full median, look at the 75%-ile. It's
			// going to be cheaper than the 50%-ile, but it still got into a
			// block.
			if uint64(progress) > types.BlockSizeLimit/4 {
				tp.recentMedians = append(tp.recentMedians, fees[i].fee)
				break
			}
		}

		// If there are more than 10 blocks recorded in the txnsPerBlock, strip
		// off the oldest blocks.
		for len(tp.recentMedians) > blockFeeEstimationDepth {
			tp.recentMedians = tp.recentMedians[1:]
		}
	}
	// Grab the median of the recent medians. Copy to a new slice so the sorting
	// doesn't screw up the slice.
	safeMedians := make([]types.Currency, len(tp.recentMedians))
	copy(safeMedians, tp.recentMedians)
	sort.Slice(safeMedians, func(i, j int) bool {
		return safeMedians[i].Cmp(safeMedians[j]) < 0
	})
	tp.recentMedianFee = safeMedians[len(safeMedians)/2]

	// Update all the on-disk structures.
	err := tp.putRecentConsensusChange(tp.dbTx, cc.ID)
	if err != nil {
		tp.log.Println("ERROR: could not update the recent consensus change:", err)
	}
	err = tp.putBlockHeight(tp.dbTx, tp.blockHeight)
	if err != nil {
		tp.log.Println("ERROR: could not update the block height:", err)
	}
	err = tp.putFeeMedian(tp.dbTx, medianPersist{
		RecentMedians:   tp.recentMedians,
		RecentMedianFee: tp.recentMedianFee,
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

// objectLocations are used to pinpoint the exact locations of an object in a
// transaction set so as to avoid recomputing ObjectIDs when searching for
// objects.
type objectLocation struct {
	setID   TransactionSetID
	txIndex int
}

// findObjectLocation returns the location of an object from its object id and
// true if it has been found. If the object's set or transaction is not found,
// the returned bool will be false.
func (tp *TransactionPool) findObjectLocation(oid ObjectID) ([]*objectLocation, bool) {
	setID, ok := tp.knownObjects[oid]
	if !ok {
		return nil, false
	}
	set := tp.transactionSets[setID]

	var location *objectLocation
	location.setID = setID
	// Iterate over all objects in all transactions the set until you find
	// the transaction containing this object.
	for i := 0; i < len(set); i++ {
		for _, sci := range set[i].SiacoinInputs {
			id := ObjectID(sci.ParentID)
			if id == oid {
				location.txIndex = i
				return location, true
			}
		}
		for i := range set[i].SiacoinOutputs {
			id := ObjectID(set[i].SiacoinOutputID(uint64(i)))
			if id == oid {
				location.txIndex = i
				return location, true
			}
		}
		for i := range set[i].FileContracts {
			id := ObjectID(set[i].FileContractID(uint64(i)))
			if id == oid {
				location.txIndex = i
				return location, true
			}
		}
		for _, fcr := range set[i].FileContractRevisions {
			id := ObjectID(fcr.ParentID)
			if id == oid {
				location.txIndex = i
				return location, true
			}
		}
		for _, sp := range set[i].StorageProofs {
			id := ObjectID(sp.ParentID)
			if id == oid {
				location.txIndex = i
				return location, true
			}
		}
		for _, sfi := range set[i].SiafundInputs {
			id := ObjectID(sfi.ParentID)
			if id == oid {
				location.txIndex = i
				return location, true
			}
		}
		for i := range set[i].SiafundOutputs {
			id := ObjectID(set[i].SiafundOutputID(uint64(i)))
			if id == oid {
				location.txIndex = i
				return location, true
			}
		}
	}
	return nil, false
}

func (tp *TransactionPool) removeTransaction(txn types.Transaction) {
	// removedTxnIndices is used to keep track of which need to be removed from
	// which sets. Sets are changed (and a diff is created) once all changes
	// have been finalized. objectLocations are used to pinpoint txn indices
	// quickly. removedTxnIndices are kept in sorted order.
	removedTxnIndices := make(map[TransactionSetID][]int)
	objectLocations := make(map[ObjectID][]*objectLocation)

	// removedObjects stores all object IDs of objects in transactions removed,
	// or put in the stack to be removed.
	removedObjects := make(map[ObjectID]struct{})
	// removalStack is a stack of objects from removed transactions that
	// need to be removed from other transactions.
	var removalStack []ObjectID

	// addTxnToRemovalStack adds all outputs of a transaction into the removal
	// stack.
	addTxnToRemovalStack := func(txn types.Transaction) {
		for i := range txn.SiacoinOutputs {
			id := ObjectID(txn.SiacoinOutputID(uint64(i)))
			if _, alreadyQueuedForRemoval := removedObjects[id]; !alreadyQueuedForRemoval {
				removedObjects[id] = struct{}{}
				removalStack = append(removalStack, id)
			}
		}
		for i := range txn.FileContracts {
			id := ObjectID(txn.FileContractID(uint64(i)))
			if _, alreadyQueuedForRemoval := removedObjects[id]; !alreadyQueuedForRemoval {
				removedObjects[id] = struct{}{}
				removalStack = append(removalStack, id)
			}
		}
		for _, fcr := range txn.FileContractRevisions {
			id := ObjectID(fcr.ParentID)
			if _, alreadyQueuedForRemoval := removedObjects[id]; !alreadyQueuedForRemoval {
				removedObjects[id] = struct{}{}
				removalStack = append(removalStack, id)
			}
		}
		for _, sp := range txn.StorageProofs {
			id := ObjectID(sp.ParentID)
			if _, alreadyQueuedForRemoval := removedObjects[id]; !alreadyQueuedForRemoval {
				removedObjects[id] = struct{}{}
				removalStack = append(removalStack, id)
			}
		}
		for i := range txn.SiafundOutputs {
			id := ObjectID(txn.SiafundOutputID(uint64(i)))
			if _, alreadyQueuedForRemoval := removedObjects[id]; !alreadyQueuedForRemoval {
				removedObjects[id] = struct{}{}
				removalStack = append(removalStack, id)
			}
		}
	}

	// Add the given transaction's objects to the stack.
	addTxnToRemovalStack(txn)

	for n := len(removalStack); n != 0; {
		// Pop off next object ID from the stack.
		nextObjectID := removalStack[n-1]
		removalStack = removalStack[:n-1]

		// Get the location of the object just popped from the stack.
		// TODO: this needs to be a func that gets locations from inputs that refer to
		// outputs in the stack
		nextObjectLocations, ok := objectLocations[nextObjectID]
		if !ok {
			nextObjectLocations, ok = tp.findObjectLocation(nextObjectID)
			if !ok {
				//TODO: Decide: Is this correct behavior? should this ever happen?
				continue
			}
		}

		for _, loc := range nextObjectLocations {
			set := tp.transactionSets[loc.setID]
			removedIndices := removedTxnIndices[loc.setID]

			i := sort.SearchInts(removedIndices, loc.txIndex)
			if i < len(removedIndices) && removedIndices[i] == loc.txIndex {
				// The tx index is present at removedIndices[i], therefore this
				// transaction has already been marked for removal, along with
				// any of its output objects.
				continue
			}
			// The tx index is not present in data, but i is the index where
			// it would be inserted.

			// Insert the index into the slice marking removed transactions.
			removedIndices = removedIndices[0 : len(removedIndices)+1]
			copy(removedIndices[i+1:], removedIndices[i:])
			removedIndices[i] = loc.txIndex

			// Add all output objects to the removal stack.
			addTxnToRemovalStack(set[loc.txIndex])
		}
	}

	// At this point all transaction sets which have had transactions removed,
	// will be marked in removedTxnIndices. Now we clean up each set
	// individually, creating new transaction sets following cP4P and checking
	// transaction set consistency. This can be

}
