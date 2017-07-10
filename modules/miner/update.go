package miner

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

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

// addMapElementTxns places the splitSet from a mapElement into the correct
// mapHeap.
func (m *Miner) addMapElementTxns(elem *mapElement) {
	candidateSet := elem.set

	// Check if heap for highest fee transactions has space.
	if m.blockMapHeap.size+candidateSet.size < types.BlockSizeLimit-5e3 {
		m.blockMapHeap.Push(elem)
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
			m.blockMapHeap.Push(elem)

			// Place transactions removed from block heap into
			// the overflow heap.
			for _, v := range bottomSets {
				m.overflowMapHeap.Push(v)
			}
			break
		}

		// If the blockMapHeap is empty, push all elements removed from it back
		// in, and place the candidate set into the overflow. This should never
		// happen since transaction sets are much smaller than the max block
		// size.
		_, exists := m.blockMapHeap.Peek()
		if !exists {
			m.overflowMapHeap.Push(elem)
			// Put back in transactions removed.
			for _, v := range bottomSets {
				m.blockMapHeap.Push(v)
			}
			// Finished with this candidate set.
			break
		}
		// Add the set to the bottomSets slice. Note that we don't increase
		// sizeOfBottomSets until after calculating the average.
		nextSet := m.blockMapHeap.Pop()
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
			m.overflowMapHeap.Push(elem)
			// Put transaction sets from bottom back into the blockMapHeap.
			for _, v := range bottomSets {
				m.blockMapHeap.Push(v)
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
		m.addMapElementTxns(newElements[i])
	}
}

// Change the UnsolvedBlock so that it  has exactly those transactions in the
// blockMapHeap.
func (m *Miner) adjustUnsolvedBlock() {
	numTxns := 0
	for _, elem := range m.blockMapHeap.selectID {
		numTxns += len(elem.set.transactions)
	}
	// If the transactions that need to be added don't fit in the block,
	// increase the size of the block by a constant factor to be more efficient.
	if numTxns > cap(m.persist.UnsolvedBlock.Transactions) {
		newCap := cap(m.persist.UnsolvedBlock.Transactions) * 6 / 5
		if numTxns > newCap {
			newCap = numTxns
		}
		m.persist.UnsolvedBlock.Transactions = make([]types.Transaction, 0, newCap)
	} else {
		m.persist.UnsolvedBlock.Transactions = m.persist.UnsolvedBlock.Transactions[:0]
	}

	// The current design removes all transactions from the block itself, so we
	// have to take everything the blockMapHeap and put it into the unsolved
	// block slice.
	for _, elem := range m.blockMapHeap.selectID {
		set := elem.set
		m.persist.UnsolvedBlock.Transactions = append(m.persist.UnsolvedBlock.Transactions, set.transactions...)
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
		}
		delete(m.fullSets, id)
	}
}

// deleteMapElementTxns removes a splitSet (by id) from the miner's mapheaps and
// readjusts the mapheap for the block if needed.
func (m *Miner) deleteMapElementTxns(id splitSetID) {
	_, inBlockMapHeap := m.blockMapHeap.selectID[id]
	_, inOverflowMapHeap := m.overflowMapHeap.selectID[id]

	// If the transaction set is in the overflow, we can just delete it.
	if inOverflowMapHeap {
		m.overflowMapHeap.RemoveSetByID(id)
	} else if inBlockMapHeap {
		// Remove from blockMapHeap.
		m.blockMapHeap.RemoveSetByID(id)

		// Promote sets from overflow heap to block if possible.
		for overflowElem, canPromote := m.overflowMapHeap.Peek(); canPromote && m.blockMapHeap.size+overflowElem.set.size < types.BlockSizeLimit-5e3; {
			promotedElem := m.overflowMapHeap.Pop()
			m.blockMapHeap.Push(promotedElem)
		}
	}
	delete(m.splitSets, id)
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
	m.adjustUnsolvedBlock()
}
