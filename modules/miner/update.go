package miner

import (
	"sort"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// addMapElementTxns places the splitSet from a mapElement into the correct
// mapHeap.
func (m *Miner) addMapElementTxns(elem *mapElement) {
	candidateSet := elem.set

	// Check if heap for highest fee transactions has space.
	if m.blockMapHeap.size+candidateSet.size < types.BlockSizeLimit-5e3 {
		m.pushToBlock(elem)
		return
	}

	// While the heap cannot fit this set s, and while the (weighted) average
	// fee for the lowest sets from the block is less than the fee for the set
	// s, continue removing from the heap. The block heap doesn't have enough
	// space for this transaction. Check if removing sets from the blockMapHeap
	// will be worth it. bottomSets will hold  the lowest fee sets from the
	// blockMapHeap
	bottomSets := make([]*mapElement, 0)
	var sizeOfBottomSets uint64
	var averageFeeOfBottomSets types.Currency
	for {
		// Check if the candidateSet can fit in the block.
		if m.blockMapHeap.size-sizeOfBottomSets+candidateSet.size < types.BlockSizeLimit-5e3 {
			// Place candidate into block,
			m.pushToBlock(elem)
			// Place transactions removed from block heap into
			// the overflow heap.
			for _, v := range bottomSets {
				m.pushToOverflow(v)
			}
			break
		}

		// If the blockMapHeap is empty, push all elements removed from it back
		// in, and place the candidate set into the overflow. This should never
		// happen since transaction sets are much smaller than the max block
		// size.
		_, exists := m.blockMapHeap.peek()
		if !exists {
			m.pushToOverflow(elem)
			// Put back in transactions removed.
			for _, v := range bottomSets {
				m.pushToBlock(v)
			}
			// Finished with this candidate set.
			break
		}
		// Add the set to the bottomSets slice. Note that we don't increase
		// sizeOfBottomSets until after calculating the average.
		nextSet := m.popFromBlock()
		bottomSets = append(bottomSets, nextSet)

		// Calculating fees to compare total fee from those sets removed and the current set s.
		totalFeeFromNextSet := nextSet.set.averageFee.Mul64(nextSet.set.size)
		totalBottomFees := averageFeeOfBottomSets.Mul64(sizeOfBottomSets).Add(totalFeeFromNextSet)
		sizeOfBottomSets += nextSet.set.size
		averageFeeOfBottomSets := totalBottomFees.Div64(sizeOfBottomSets)

		// If the average fee of the bottom sets from the block is higher than
		// the fee from this candidate set, put the candidate into the overflow
		// MapHeap.
		if averageFeeOfBottomSets.Cmp(candidateSet.averageFee) == 1 {
			// CandidateSet goes into the overflow.
			m.pushToOverflow(elem)
			// Put transaction sets from bottom back into the blockMapHeap.
			for _, v := range bottomSets {
				m.pushToBlock(v)
			}
			// Finished with this candidate set.
			break
		}
	}
}

// addNewTxns adds new unconfirmed transactions to the miner's transaction
// selection and updates the splitSet and mapElement state of the miner.
func (m *Miner) addNewTxns(diff *modules.TransactionPoolDiff) {
	// Get new splitSets (in form of mapElement)
	newElements := m.getNewSplitSets(diff)

	// Place each elem in one of the MapHeaps.
	for i := 0; i < len(newElements); i++ {
		// Add splitSet to miner's global state using pointer and ID stored in
		// the mapElement and then add the mapElement to the miner's global
		// state.
		m.splitSets[newElements[i].id] = newElements[i].set
		for _, tx := range newElements[i].set.transactions {
			m.splitSetIDFromTxID[tx.ID()] = newElements[i].id
		}
		m.addMapElementTxns(newElements[i])
	}
}

// deleteMapElementTxns removes a splitSet (by id) from the miner's mapheaps and
// readjusts the mapheap for the block if needed.
func (m *Miner) deleteMapElementTxns(id splitSetID) {
	_, inBlockMapHeap := m.blockMapHeap.selectID[id]
	_, inOverflowMapHeap := m.overflowMapHeap.selectID[id]

	// If the transaction set is in the overflow, we can just delete it.
	if inOverflowMapHeap {
		m.overflowMapHeap.removeSetByID(id)
	} else if inBlockMapHeap {
		// Remove from blockMapHeap.
		m.blockMapHeap.removeSetByID(id)
		m.removeSplitSetFromUnsolvedBlock(id)

		// Promote sets from overflow heap to block if possible.
		for overflowElem, canPromote := m.peekAtOverflow(); canPromote && m.blockMapHeap.size+overflowElem.set.size < types.BlockSizeLimit-5e3; {
			promotedElem := m.popFromOverflow()
			m.pushToBlock(promotedElem)
		}
	}
}

// deleteReverts deletes transactions from the miner's transaction selection
// which are no longer in the transaction pool.
func (m *Miner) deleteReverts(diff *modules.TransactionPoolDiff) {
	// Delete the sets that are no longer useful. That means recognizing which
	// of your splits belong to the missing sets.
	for _, id := range diff.RevertedTransactions {
		// Look up all of the split sets associated with the set being reverted,
		// and delete them. Then delete the lookups from the list of full sets
		// as well.
		splitSetIndexes := m.fullSets[id]
		for _, ss := range splitSetIndexes {
			m.deleteMapElementTxns(splitSetID(ss))
			delete(m.splitSets, splitSetID(ss))
		}
		delete(m.fullSets, id)
	}
}

// fixSplitSetOrdering maintains the relative ordering of transactions from a
// split set within the block.
func (m *Miner) fixSplitSetOrdering(id splitSetID) {
	set, _ := m.splitSets[id]
	setTxs := set.transactions
	var setTxIDs []types.TransactionID
	var setTxIndices []int // These are the indices within the unsolved block.

	// No swapping necessary if there are less than 2 transactions in the set.
	if len(setTxs) < 2 {
		return
	}

	// Iterate over all transactions in the set and store their txIDs and their
	// indices within the unsoved block.
	for i := 0; i < len(setTxs); i++ {
		txID := setTxs[i].ID()
		setTxIDs = append(setTxIDs, txID)
		setTxIndices = append(setTxIndices, m.unsolvedBlockIndex[txID])
	}

	// Sort the indices and maintain the sets relative ordering in the block by
	// changing their positions if necessary. The ordering within the set should
	// be exactly the order in which the sets appear in the block.
	sort.Ints(setTxIndices)
	for i := 0; i < len(setTxIDs); i++ {
		index := m.unsolvedBlockIndex[setTxIDs[i]]
		expectedIndex := setTxIndices[i]
		// Put the transaction in the correct position in the block.
		if index != expectedIndex {
			m.persist.UnsolvedBlock.Transactions[expectedIndex] = setTxs[i]
			m.unsolvedBlockIndex[setTxIDs[i]] = expectedIndex
		}
	}
}

// getNewSplitSets creates split sets from a transaction pool diff, returns them
// in a slice of map elements. Does not update the miner's global state.
func (m *Miner) getNewSplitSets(diff *modules.TransactionPoolDiff) []*mapElement {
	// Split the new sets and add the splits to the list of transactions we pull
	// form.
	newElements := make([]*mapElement, 0)
	for _, newSet := range diff.AppliedTransactions {
		// Split the sets into smaller sets, and add them to the list of
		// transactions the miner can draw from.
		// TODO: Split the one set into a bunch of smaller sets using the cp4p
		// splitter.
		m.setCounter++
		m.fullSets[newSet.ID] = []int{m.setCounter}
		var size uint64
		var totalFees types.Currency
		for i := range newSet.IDs {
			size += newSet.Sizes[i]
			for _, fee := range newSet.Transactions[i].MinerFees {
				totalFees = totalFees.Add(fee)
			}
		}
		// We will check to see if this splitSet belongs in the block.
		s := &splitSet{
			size:         size,
			averageFee:   totalFees.Div64(size),
			transactions: newSet.Transactions,
		}

		elem := &mapElement{
			set:   s,
			id:    splitSetID(m.setCounter),
			index: 0,
		}
		newElements = append(newElements, elem)
	}
	return newElements
}

// peekAtBlock checks top of the blockMapHeap, and returns the top element (but
// does not remove it from the heap). Returns false if the heap is empty.
func (m *Miner) peekAtBlock() (*mapElement, bool) {
	return m.blockMapHeap.peek()
}

// peekAtOverflow checks top of the overflowMapHeap, and returns the top element
// (but does not remove it from the heap). Returns false if the heap is empty.
func (m *Miner) peekAtOverflow() (*mapElement, bool) {
	return m.overflowMapHeap.peek()
}

// popFromBlock pops an element from the blockMapHeap, removes it from the
// miner's unsolved block, and maintains proper set ordering within the block.
func (m *Miner) popFromBlock() *mapElement {
	elem := m.blockMapHeap.pop()
	m.removeSplitSetFromUnsolvedBlock(elem.id)
	return elem
}

// popFromBlock pops an element from the overflowMapHeap.
func (m *Miner) popFromOverflow() *mapElement {
	return m.overflowMapHeap.pop()
}

// pushToBlock pushes a mapElement onto the blockMapHeap and appends it to the
// unsolved block in the miner's global state.
func (m *Miner) pushToBlock(elem *mapElement) {
	m.blockMapHeap.push(elem)
	transactions := elem.set.transactions

	// Place the transactions from this set into the block and store their indices.
	for i := 0; i < len(transactions); i++ {
		m.unsolvedBlockIndex[transactions[i].ID()] = len(m.persist.UnsolvedBlock.Transactions)
		m.persist.UnsolvedBlock.Transactions = append(m.persist.UnsolvedBlock.Transactions, transactions[i])
	}
}

// pushToOverflow pushes a mapElement onto the overflowMapHeap.
func (m *Miner) pushToOverflow(elem *mapElement) {
	m.overflowMapHeap.push(elem)
}

// ProcessConsensusDigest will update the miner's most recent block.
func (m *Miner) ProcessConsensusChange(cc modules.ConsensusChange) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Update the miner's understanding of the block height.
	for _, block := range cc.RevertedBlocks {
		// Only doing the block check if the height is above zero saves hashing
		// and saves a nontrivial amount of time during IBD.
		if m.persist.Height > 0 || block.ID() != types.GenesisID {
			m.persist.Height--
		} else if m.persist.Height != 0 {
			// Sanity check - if the current block is the genesis block, the
			// miner height should be set to zero.
			m.log.Critical("Miner has detected a genesis block, but the height of the miner is set to ", m.persist.Height)
			m.persist.Height = 0
		}
	}
	for _, block := range cc.AppliedBlocks {
		// Only doing the block check if the height is above zero saves hashing
		// and saves a nontrivial amount of time during IBD.
		if m.persist.Height > 0 || block.ID() != types.GenesisID {
			m.persist.Height++
		} else if m.persist.Height != 0 {
			// Sanity check - if the current block is the genesis block, the
			// miner height should be set to zero.
			m.log.Critical("Miner has detected a genesis block, but the height of the miner is set to ", m.persist.Height)
			m.persist.Height = 0
		}
	}

	// Update the unsolved block.
	m.persist.UnsolvedBlock.ParentID = cc.AppliedBlocks[len(cc.AppliedBlocks)-1].ID()
	m.persist.Target = cc.ChildTarget
	m.persist.UnsolvedBlock.Timestamp = cc.MinimumValidChildTimestamp

	// There is a new parent block, the source block should be updated to keep
	// the stale rate as low as possible.
	if cc.Synced {
		m.newSourceBlock()
	}
	m.persist.RecentChange = cc.ID
}

// ReceiveUpdatedUnconfirmedTransactions will replace the current unconfirmed
// set of transactions with the input transactions.
func (m *Miner) ReceiveUpdatedUnconfirmedTransactions(diff *modules.TransactionPoolDiff) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.deleteReverts(diff)
	m.addNewTxns(diff)
}

// removeSplitSetFromUnsolvedBlock removes a split set from the miner's unsolved
// block.
func (m *Miner) removeSplitSetFromUnsolvedBlock(id splitSetID) {
	transactions := m.splitSets[id].transactions
	// swappedTxs stores transaction IDs for all transactions that are swapped
	// during the process of removing this splitSet.
	swappedTxs := make(map[types.TransactionID]struct{})

	// Remove each transaction from this set from the block and track the
	// transactions that were moved during that action.
	for i := 0; i < len(transactions); i++ {
		txID := m.removeTxFromUnsolvedBlock(transactions[i].ID())
		swappedTxs[txID] = struct{}{}
	}

	// setsFixed keeps track of the splitSets which contain swapped transactions
	// and have been checked for having the correct set ordering.
	setsFixed := make(map[splitSetID]struct{})
	// Iterate over all swapped transactions and fix the ordering of their set
	// if necessary.
	for txID := range swappedTxs {
		setID, _ := m.splitSetIDFromTxID[txID]
		_, thisSetFixed := setsFixed[setID]

		// If this set was already fixed, or if the transaction is from the set
		// being removed we can move on to the next transaction.
		if thisSetFixed || setID == id {
			continue
		}

		// Fix the set ordering and add the splitSet to the set of fixed sets.
		m.fixSplitSetOrdering(setID)
		setsFixed[setID] = struct{}{}
	}
}

// removeTxFromUnsolvedBlock removes the given transaction by either swapping it
// with the transaction at the end of the slice or, if the transaction to be
// removed is the last transaction in the block, just shrinking the slice. It
// returns the transaction ID of the last element in the block prior to the
// swap/removal taking place.
func (m *Miner) removeTxFromUnsolvedBlock(id types.TransactionID) types.TransactionID {
	index, _ := m.unsolvedBlockIndex[id]
	length := len(m.persist.UnsolvedBlock.Transactions)
	// Remove this transactionID from the map of indices.
	delete(m.unsolvedBlockIndex, id)

	// If the transaction is already the last transaction in the block, we can
	// remove it by just shrinking the block.
	if index == length-1 {
		m.persist.UnsolvedBlock.Transactions = m.persist.UnsolvedBlock.Transactions[:length-1]
		return id
	}

	lastTx := m.persist.UnsolvedBlock.Transactions[length-1]
	lastTxID := lastTx.ID()
	// Swap with the last transaction in the slice, change the miner state to
	// match the new index, and shrink the slice by 1 space.
	m.persist.UnsolvedBlock.Transactions[index] = lastTx
	m.unsolvedBlockIndex[lastTxID] = index
	m.persist.UnsolvedBlock.Transactions = m.persist.UnsolvedBlock.Transactions[:length-1]
	return lastTxID
}
