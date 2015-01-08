package miner

import (
	"encoding/json"

	"github.com/NebulousLabs/Sia/consensus"
)

// MinerStatus is the information that gets returned to the front end. Each
// item is returned in the format that it's meant to be displayed.
type MinerStatus struct {
	State          string
	Threads        int
	RunningThreads int
	Address        consensus.CoinAddress
}

// Info() returns a JSON struct which can be parsed by frontends for displaying
// information to the user.
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
func (m *Miner) Info() ([]byte, error) {
	m.rLock()
	defer m.rUnlock()

	status := MinerStatus{
		Threads:        m.threads,
		RunningThreads: m.runningThreads,
		Address:        m.address,
	}

	// Set the running status based on desiredThreads vs. runningThreads.
	if m.desiredThreads == 0 && m.runningThreads == 0 {
		status.State = "Off"
	} else if m.desiredThreads == 0 && m.runningThreads > 0 {
		status.State = "Turning Off"
	} else if m.desiredThreads == m.runningThreads {
		status.State = "On"
	} else if m.desiredThreads < m.runningThreads {
		status.State = "Turning On"
	} else if m.desiredThreads > m.runningThreads {
		status.State = "Decreasing number of threads."
	} else {
		status.State = "Miner is in an ERROR state!"
	}

	return json.Marshal(status)
}

// Threads returns the number of threads being used by the miner.
func (m *Miner) Threads() int {
	m.rLock()
	defer m.rUnlock()
	return m.threads
}

// SubsidyAddress returns the address that is currently being used by the miner
// while searching for blocks.
func (m *Miner) SubsidyAddress() consensus.CoinAddress {
	m.lock()
	defer m.unlock()
	return m.address
}
