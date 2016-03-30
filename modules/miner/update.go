package miner

import (
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// ProcessConsensusDigest will update the miner's most recent block.
func (m *Miner) ProcessConsensusChange(cc modules.ConsensusChange) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, block := range cc.RevertedBlocks {
		if block.ID() != types.GenesisBlock.ID() {
			m.persist.Height--
		} else if m.persist.Height != 0 {
			// Sanity check - if the current block is the genesis block, the
			// miner height should be set to zero.
			m.log.Critical("Miner has detected a genesis block, but the height of the miner is set to ", m.persist.Height)
			m.persist.Height = 0
		}
	}
	for _, block := range cc.AppliedBlocks {
		if block.ID() != types.GenesisBlock.ID() {
			m.persist.Height++
		} else if m.persist.Height != 0 {
			// Sanity check - if the current block is the genesis block, the
			// miner height should be set to zero.
			m.log.Critical("Miner has detected a genesis block, but the height of the miner is set to ", m.persist.Height)
			m.persist.Height = 0
		}
	}
	// Sanity check - if the most recent block in the miner is the same as the
	// most recent block in the consensus set, then the height of the consensus
	// set and the height of the miner should be the same.
	if cc.AppliedBlocks[len(cc.AppliedBlocks)-1].ID() == m.cs.CurrentBlock().ID() {
		if m.persist.Height != m.cs.Height() {
			m.log.Critical("Miner has a height mismatch: expecting ", m.cs.Height(), " but got ", m.persist.Height, ". Recent update had ", len(cc.RevertedBlocks), " reverted blocks, and ", len(cc.AppliedBlocks), " applied blocks.")
			m.persist.Height = m.cs.Height()
		}
	}

	// Update the unsolved block.
	var exists1, exists2 bool
	m.persist.UnsolvedBlock.ParentID = cc.AppliedBlocks[len(cc.AppliedBlocks)-1].ID()
	m.persist.Target, exists1 = m.cs.ChildTarget(m.persist.UnsolvedBlock.ParentID)
	m.persist.UnsolvedBlock.Timestamp, exists2 = m.cs.MinimumValidChildTimestamp(m.persist.UnsolvedBlock.ParentID)
	if !exists1 {
		m.log.Critical("miner was unable to find parent id of an unsolved block in the consensus set")
	}
	if !exists2 {
		m.log.Critical("miner was unable to find child timestamp of an unsovled block in the consensus set")
	}

	// There is a new parent block, the source block should be updated to keep
	// the stale rate as low as possible.
	m.newSourceBlock()
	m.persist.RecentChange = cc.ID
}

// ReceiveUpdatedUnconfirmedTransactions will replace the current unconfirmed
// set of transactions with the input transactions.
func (m *Miner) ReceiveUpdatedUnconfirmedTransactions(unconfirmedTransactions []types.Transaction, _ modules.ConsensusChange) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Edge case - if there are no transactions, set the block's transactions
	// to nil and return.
	if len(unconfirmedTransactions) == 0 {
		m.persist.UnsolvedBlock.Transactions = nil
		return
	}

	// Add transactions to the block until the block size limit is reached.
	// Transactions are assumed to be in a sensible order.
	var i int
	remainingSize := int(types.BlockSizeLimit - 5e3)
	for i = range unconfirmedTransactions {
		remainingSize -= len(encoding.Marshal(unconfirmedTransactions[i]))
		if remainingSize < 0 {
			break
		}
	}
	m.persist.UnsolvedBlock.Transactions = unconfirmedTransactions[:i+1]
}
