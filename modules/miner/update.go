package miner

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// ProcessConsensusDigest will update the miner's most recent block.
func (m *Miner) ProcessConsensusDigest(revertedIDs, appliedIDs []types.BlockID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Sanity check - the length of appliedIDs should always be non-zero.
	if build.DEBUG && len(appliedIDs) == 0 {
		panic("received a digest with no applied blocks")
	}

	// Adjust the height of the miner.
	m.height -= types.BlockHeight(len(revertedIDs))
	m.height += types.BlockHeight(len(appliedIDs))

	// Update the unsolved block.
	var exists1, exists2 bool
	m.unsolvedBlock.ParentID = appliedIDs[len(appliedIDs)-1]
	m.target, exists1 = m.cs.ChildTarget(m.unsolvedBlock.ParentID)
	m.unsolvedBlock.Timestamp, exists2 = m.cs.MinimumValidChildTimestamp(m.unsolvedBlock.ParentID)
	if build.DEBUG && !exists1 {
		panic("could not get child target")
	}
	if build.DEBUG && !exists2 {
		panic("could not get child earliest timestamp")
	}

	// There is a new parent block, the source block should be updated to keep
	// the stale rate as low as possible.
	m.newSourceBlock()
}

// ReceiveUpdatedUnconfirmedTransactions will replace the current unconfirmed
// set of transactions with the input transactions.
func (m *Miner) ReceiveUpdatedUnconfirmedTransactions(unconfirmedTransactions []types.Transaction, _ modules.ConsensusChange) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Edge case - if there are no transactions, set the block's transactions
	// to nil and return.
	if len(unconfirmedTransactions) == 0 {
		m.unsolvedBlock.Transactions = nil
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
	m.unsolvedBlock.Transactions = unconfirmedTransactions[0 : i+1]
}
