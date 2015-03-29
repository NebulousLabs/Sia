package miner

import (
	"github.com/NebulousLabs/Sia/consensus"
)

// ReceiveTransactionPoolUpdate listens to the transaction pool for changes in
// the transaction pool. These changes will be applied to the blocks being
// mined.
func (m *Miner) ReceiveTransactionPoolUpdate(revertedBlocks, appliedBlocks []consensus.Block, unconfirmedTransactions []consensus.Transaction, unconfirmedSiacoinOutputDiffs []consensus.SiacoinOutputDiff) {
	m.mu.Lock()
	defer m.mu.Unlock()
	defer m.notifySubscribers()
	m.stateHeight -= consensus.BlockHeight(len(revertedBlocks))
	m.stateHeight += consensus.BlockHeight(len(appliedBlocks))
	m.transactions = unconfirmedTransactions

	id := m.state.RLock()
	defer m.state.RUnlock(id)
	if m.stateHeight != m.state.Height() {
		return
	}
	m.parent = m.state.CurrentBlock().ID()
	m.target = m.state.CurrentTarget()
	m.earliestTimestamp = m.state.EarliestTimestamp()
}
