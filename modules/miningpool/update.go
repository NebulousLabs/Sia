package pool

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// getNewSplitSets creates split sets from a transaction pool diff, returns them
// in a slice of map elements. Does not update the pool's global state.
func (p *Pool) getNewSplitSets(diff *modules.TransactionPoolDiff) []*mapElement {
	// Split the new sets and add the splits to the list of transactions we pull
	// form.
	newElements := make([]*mapElement, 0)
	for _, newSet := range diff.AppliedTransactions {
		// Split the sets into smaller sets, and add them to the list of
		// transactions the pool can draw from.
		// TODO: Split the one set into a bunch of smaller sets using the cp4p
		// splitter.
		p.setCounter++
		p.fullSets[newSet.ID] = []int{p.setCounter}
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
			id:    splitSetID(p.setCounter),
			index: 0,
		}
		newElements = append(newElements, elem)
	}
	return newElements
}

// addMapElementTxns places the splitSet from a mapElement into the correct
// mapHeap.
func (p *Pool) addMapElementTxns(elem *mapElement) {
	candidateSet := elem.set

	// Check if heap for highest fee transactions has space.
	if p.blockMapHeap.size+candidateSet.size < types.BlockSizeLimit-5e3 {
		p.blockMapHeap.push(elem)
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
		if p.blockMapHeap.size-sizeOfBottomSets+candidateSet.size < types.BlockSizeLimit-5e3 {
			// Place candidate into block,
			p.blockMapHeap.push(elem)

			// Place transactions removed from block heap into
			// the overflow heap.
			for _, v := range bottomSets {
				p.overflowMapHeap.push(v)
			}
			break
		}

		// If the blockMapHeap is empty, push all elements removed from it back
		// in, and place the candidate set into the overflow. This should never
		// happen since transaction sets are much smaller than the max block
		// size.
		_, exists := p.blockMapHeap.peek()
		if !exists {
			p.overflowMapHeap.push(elem)
			// Put back in transactions removed.
			for _, v := range bottomSets {
				p.blockMapHeap.push(v)
			}
			// Finished with this candidate set.
			break
		}
		// Add the set to the bottomSets slice. Note that we don't increase
		// sizeOfBottomSets until after calculating the average.
		nextSet := p.blockMapHeap.pop()
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
			p.overflowMapHeap.push(elem)
			// Put transaction sets from bottom back into the blockMapHeap.
			for _, v := range bottomSets {
				p.blockMapHeap.push(v)
			}
			// Finished with this candidate set.
			break
		}
	}
}

// addNewTxns adds new unconfirmed transactions to the pool's transaction
// selection and updates the splitSet and mapElement state of the pool.
func (p *Pool) addNewTxns(diff *modules.TransactionPoolDiff) {
	// Get new splitSets (in form of mapElement)
	newElements := p.getNewSplitSets(diff)

	// Place each elem in one of the MapHeaps.
	for i := 0; i < len(newElements); i++ {
		// Add splitSet to pool's global state using pointer and ID stored in
		// the mapElement and then add the mapElement to the pool's global
		// state.
		p.splitSets[newElements[i].id] = newElements[i].set
		p.addMapElementTxns(newElements[i])
	}
}

// Change the UnsolvedBlock so that it  has exactly those transactions in the
// blockMapHeap.
func (p *Pool) adjustUnsolvedBlock() {
	numTxns := 0
	for _, elem := range p.blockMapHeap.selectID {
		numTxns += len(elem.set.transactions)
	}
	// If the transactions that need to be added don't fit in the block,
	// increase the size of the block by a constant factor to be more efficient.
	if numTxns > cap(p.persist.UnsolvedBlock.Transactions) {
		newCap := cap(p.persist.UnsolvedBlock.Transactions) * 6 / 5
		if numTxns > newCap {
			newCap = numTxns
		}
		p.persist.UnsolvedBlock.Transactions = make([]types.Transaction, 0, newCap)
	} else {
		p.persist.UnsolvedBlock.Transactions = p.persist.UnsolvedBlock.Transactions[:0]
	}

	// The current design removes all transactions from the block itself, so we
	// have to take everything the blockMapHeap and put it into the unsolved
	// block slice.
	for _, elem := range p.blockMapHeap.selectID {
		set := elem.set
		p.persist.UnsolvedBlock.Transactions = append(p.persist.UnsolvedBlock.Transactions, set.transactions...)
	}
}

// deleteReverts deletes transactions from the mining pool's transaction selection
// which are no longer in the transaction pool.
func (p *Pool) deleteReverts(diff *modules.TransactionPoolDiff) {
	// Delete the sets that are no longer useful. That means recognizing which
	// of your splits belong to the missing sets.
	for _, id := range diff.RevertedTransactions {
		// Look up all of the split sets associated with the set being reverted,
		// and delete them. Then delete the lookups from the list of full sets
		// as well.
		splitSetIndexes := p.fullSets[id]
		for _, ss := range splitSetIndexes {
			p.deleteMapElementTxns(splitSetID(ss))
		}
		delete(p.fullSets, id)
	}
}

// deleteMapElementTxns removes a splitSet (by id) from the pool's mapheaps and
// readjusts the mapheap for the block if needed.
func (p *Pool) deleteMapElementTxns(id splitSetID) {
	_, inBlockMapHeap := p.blockMapHeap.selectID[id]
	_, inOverflowMapHeap := p.overflowMapHeap.selectID[id]

	// If the transaction set is in the overflow, we can just delete it.
	if inOverflowMapHeap {
		p.overflowMapHeap.removeSetByID(id)
	} else if inBlockMapHeap {
		// Remove from blockMapHeap.
		p.blockMapHeap.removeSetByID(id)

		// Promote sets from overflow heap to block if possible.
		for overflowElem, canPromote := p.overflowMapHeap.peek(); canPromote && p.blockMapHeap.size+overflowElem.set.size < types.BlockSizeLimit-5e3; {
			promotedElem := p.overflowMapHeap.pop()
			p.blockMapHeap.push(promotedElem)
		}
	}
	delete(p.splitSets, id)
}

// ProcessConsensusChange will update the pool's most recent block.
func (p *Pool) ProcessConsensusChange(cc modules.ConsensusChange) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Update the pool's understanding of the block height.
	for _, block := range cc.RevertedBlocks {
		// Only doing the block check if the height is above zero saves hashing
		// and saves a nontrivial amount of time during IBD.
		if p.persist.BlockHeight > 0 || block.ID() != types.GenesisID {
			p.persist.BlockHeight--
		} else if p.persist.BlockHeight != 0 {
			// Sanity check - if the current block is the genesis block, the
			// pool height should be set to zero.
			p.log.Critical("Pool has detected a genesis block, but the height of the pool is set to ", p.persist.BlockHeight)
			p.persist.BlockHeight = 0
		}
	}
	for _, block := range cc.AppliedBlocks {
		// Only doing the block check if the height is above zero saves hashing
		// and saves a nontrivial amount of time during IBD.
		if p.persist.BlockHeight > 0 || block.ID() != types.GenesisID {
			p.persist.BlockHeight++
		} else if p.persist.BlockHeight != 0 {
			// Sanity check - if the current block is the genesis block, the
			// pool height should be set to zero.
			p.log.Critical("Pool has detected a genesis block, but the height of the pool is set to ", p.persist.BlockHeight)
			p.persist.BlockHeight = 0
		}
	}

	// Update the unsolved block.
	p.persist.UnsolvedBlock.ParentID = cc.AppliedBlocks[len(cc.AppliedBlocks)-1].ID()
	p.persist.Target = cc.ChildTarget
	p.persist.UnsolvedBlock.Timestamp = cc.MinimumValidChildTimestamp
	p.persist.RecentChange = cc.ID

	// There is a new parent block, the source block should be updated to keep
	// the stale rate as low as possible.
	if cc.Synced {
		p.newSourceBlock()
	}
	p.persist.RecentChange = cc.ID
}

// ReceiveUpdatedUnconfirmedTransactions will replace the current unconfirmed
// set of transactions with the input transactions.
func (p *Pool) ReceiveUpdatedUnconfirmedTransactions(diff *modules.TransactionPoolDiff) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.deleteReverts(diff)
	p.addNewTxns(diff)
	p.adjustUnsolvedBlock()
}
