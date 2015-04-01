package miner

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
)

// ReceiveTransactionPoolUpdate listens to the transaction pool for changes in
// the transaction pool. These changes will be applied to the blocks being
// mined.
func (m *Miner) ReceiveTransactionPoolUpdate(revertedBlocks, appliedBlocks []consensus.Block, unconfirmedTransactions []consensus.Transaction, unconfirmedSiacoinOutputDiffs []consensus.SiacoinOutputDiff) {
	m.mu.Lock()
	defer m.mu.Unlock()
	defer m.notifySubscribers()

	// The total encoded size of the transactions cannot exceed the block size.
	m.transactions = nil
	remainingSize := int(consensus.BlockSizeLimit - 5e3)
	for {
		if len(unconfirmedTransactions) == 0 {
			break
		}
		remainingSize -= len(encoding.Marshal(unconfirmedTransactions[0]))
		if remainingSize < 0 {
			break
		}

		m.transactions = append(m.transactions, unconfirmedTransactions[0])
		unconfirmedTransactions = unconfirmedTransactions[1:]
	}

	// If no blocks have been applied, the block variables do not need to be
	// updated.
	if len(appliedBlocks) == 0 {
		if consensus.DEBUG {
			if len(revertedBlocks) != 0 {
				panic("blocks reverted without being added")
			}
		}
		return
	}

	// Update the parent, target, and earliest timestamp fields for the miner.
	m.parent = appliedBlocks[len(appliedBlocks)-1].ID()
	target, exists1 := m.state.ChildTarget(m.parent)
	timestamp, exists2 := m.state.EarliestChildTimestamp(m.parent)
	if consensus.DEBUG {
		if !exists1 {
			panic("could not get child target")
		}
		if !exists2 {
			panic("could not get child earliest timestamp")
		}
	}
	m.target = target
	m.earliestTimestamp = timestamp
}
