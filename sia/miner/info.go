package miner

import (
	"encoding/json"

	"github.com/NebulousLabs/Sia/consensus"
)

type Status struct {
	State          string
	Threads        int
	RunningThreads int
	Address        consensus.CoinAddress
}

// Info() returns a JSON struct which can be parsed by frontends for displaying
// information to the user.
func (m *Miner) Info() ([]byte, error) {
	m.rLock()
	defer m.rUnlock()

	status := Status{
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
