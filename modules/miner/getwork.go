package miner

import (
	"github.com/NebulousLabs/Sia/modules"
)

// GetWork() returns a MinerWork struct which can be converted to JSON to be
// parsed by external miners
func (m *Miner) GetWork() modules.MinerWork {
	m.mu.Lock()
	b := m.blockForWork()
	target := m.target
	m.mu.Unlock()

	work := modules.MinerWork{
		Block:      b,
		ParentID:   b.ParentID,
		Nonce:      b.Nonce,
		MerkleRoot: b.MerkleRoot(),
		Target:     target,
	}

	return work
}
