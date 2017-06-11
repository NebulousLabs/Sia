package miner

import (
	"sort"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

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
func (m *Miner) ReceiveUpdatedUnconfirmedTransactions(diffs []*modules.TransactionSetDiff) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Delete the sets that are no longer useful. That means recognizing which
	// of your splits belong to the missing sets.
	for _, diff := range diffs {
		// Break if this diff is not revert, as there will be no more reverted
		// diffs after this point.
		if diff.Direction != modules.DiffRevert {
			break
		}

		// Look up all of the split sets associated with the set being reverted,
		// and delete them. Then delete the lookups from the list of full sets
		// as well.
		splitSetIndexes := m.fullSets[diff.ID]
		for _, ss := range splitSetIndexes {
			delete(m.splitSets, ss)
		}
		delete(m.fullSets, diff.ID)
	}

	// Split the new sets and add the splits to the list of transactions we pull
	// form.
	for _, diff := range diffs {
		// Skip the reverted diffs, we only care about the applied ones.
		if diff.Direction == modules.DiffRevert {
			continue
		}

		// Split the sets into smaller sets, and add them to the list of
		// transactions the miner can draw from.
		//
		// TODO: Split the one set into a bunch of smaller sets using the cp4p
		// splitter.
		m.setCounter++
		m.fullSets[diff.ID] = []int{m.setCounter}
		var size int
		var totalFees types.Currency
		for _, txn := range diff.Transactions {
			size += len(encoding.Marshal(txn))
			for _, fee := range txn.MinerFees {
				totalFees = totalFees.Add(fee)
			}
		}
		m.splitSets[m.setCounter] = splitSet{
			size:         size,
			averageFee:   totalFees.Div64(uint64(size)),
			transactions: diff.Transactions,
		}
	}

	// Sort the split sets and select the BlockSizeLimit most valueable sets.
	sortedSets := make([]splitSet, 0, len(m.splitSets))
	for i := range m.splitSets {
		sortedSets = append(sortedSets, m.splitSets[i])
	}
	sort.Slice(sortedSets, func(i, j int) bool {
		return sortedSets[i].averageFee.Cmp(sortedSets[j].averageFee) < 0
	})
	var totalSize int
	m.persist.UnsolvedBlock.Transactions = nil
	for _, set := range m.splitSets {
		totalSize += set.size
		if uint64(totalSize) > types.BlockSizeLimit-5e3 {
			// There is no longer enough room to add this transction set. Stop
			// here.
			break
		}
		m.persist.UnsolvedBlock.Transactions = append(m.persist.UnsolvedBlock.Transactions, set.transactions...)
	}
}
