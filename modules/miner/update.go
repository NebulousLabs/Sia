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

	m.parent = appliedIDs[len(appliedIDs)-1]
	target, exists1 := m.cs.ChildTarget(m.parent)
	timestamp, exists2 := m.cs.EarliestChildTimestamp(m.parent)
	if build.DEBUG {
		if !exists1 {
			panic("could not get child target")
		}
		if !exists2 {
			panic("could not get child earliest timestamp")
		}
	}
	m.target = target
	m.earliestTimestamp = timestamp
	m.prepareNewBlock()
}

// ReceiveUpdatedUnconfirmedTransactions will replace the current unconfirmed
// set of transactions with the input transactions.
func (m *Miner) ReceiveUpdatedUnconfirmedTransactions(unconfirmedTransactions []types.Transaction, _ modules.ConsensusChange) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.transactions = nil
	remainingSize := int(types.BlockSizeLimit - 5e3)
	for i := range unconfirmedTransactions {
		remainingSize -= len(encoding.Marshal(unconfirmedTransactions[i]))
		if remainingSize < 0 {
			break
		}
		m.transactions = unconfirmedTransactions[0 : i+1]
	}
}
