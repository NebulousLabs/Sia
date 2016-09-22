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
	m.newSourceBlock()
	m.persist.RecentChange = cc.ID
	err := m.save()
	if err != nil {
		m.log.Println(err)
	}
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
