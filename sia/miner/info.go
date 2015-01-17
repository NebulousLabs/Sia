package miner

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/sia/components"
)

// Info() returns a MinerInfo struct which can be converted to JSON to be
// parsed by frontends for displaying information to the user.
//
// State is a string indicating what the miner is currently doing with respect
// to the number of threads it currently has vs. the number of threads it wants
// to have.
//
// Threads is the number of threads that the miner currently wants to have.
//
// RunningThreads is the number of threads that the miner currently has.
//
// Address is the current address that is receiving block payouts.
func (m *Miner) Info() (components.MinerInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info := components.MinerInfo{
		Threads:        m.threads,
		RunningThreads: m.runningThreads,
		Address:        m.address,
	}

	// Set the running info based on desiredThreads vs. runningThreads.
	if m.desiredThreads == 0 && m.runningThreads == 0 {
		info.State = "Off"
	} else if m.desiredThreads == 0 && m.runningThreads > 0 {
		info.State = "Turning Off"
	} else if m.desiredThreads == m.runningThreads {
		info.State = "On"
	} else if m.desiredThreads < m.runningThreads {
		info.State = "Turning On"
	} else if m.desiredThreads > m.runningThreads {
		info.State = "Decreasing number of threads."
	} else {
		info.State = "Miner is in an ERROR state!"
	}

	return info, nil
}

// Threads returns the number of threads being used by the miner.
func (m *Miner) Threads() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.threads
}

// SubsidyAddress returns the address that is currently being used by the miner
// while searching for blocks.
func (m *Miner) SubsidyAddress() consensus.CoinAddress {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.address
}
