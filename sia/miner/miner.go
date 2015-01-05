package miner

import (
	"encoding/json"
	"runtime"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/sia"
)

// Might want to switch to having a settings variable.
type Miner struct {
	// Block variables - helps the miner construct the next block.
	parent            consensus.BlockID
	transactions      []consensus.Transaction
	address           consensus.CoinAddress
	target            consensus.Target
	earliestTimestamp consensus.Timestamp

	threads              int // how many threads the miner usually uses.
	desiredThreads       int // 0 if not mining.
	runningThreads       int
	iterationsPerAttempt uint64

	blockChan chan consensus.Block
	sync.RWMutex
}

type Status struct {
	State          string
	Threads        int
	RunningThreads int
	Address        consensus.CoinAddress
}

// New takes a block channel down which it drops blocks that it mines. It also
// takes a thread count, which it uses to spin up miners on separate threads.
func New() (m *Miner) {
	return &Miner{
		iterationsPerAttempt: 256 * 1024,
	}
}

// Info() returns a JSON struct which can be parsed by frontends for displaying
// information to the user.
func (m *Miner) Info() ([]byte, error) {
	m.RLock()
	defer m.RUnlock()

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

// SubsidyAddress returns the address that is currently being used by the miner
// while searching for blocks.
func (m *Miner) SubsidyAddress() consensus.CoinAddress {
	m.Lock()
	defer m.Unlock()

	return m.address
}

// Update changes what block the miner is mining on. Changes include address
// and target.
func (m *Miner) Update(mu sia.MinerUpdate) error {
	m.Lock()
	defer m.Unlock()

	m.parent = mu.Parent
	m.transactions = mu.Transactions
	m.target = mu.Target
	m.address = mu.Address
	m.earliestTimestamp = mu.EarliestTimestamp

	if mu.Threads != 0 {
		m.threads = mu.Threads
		runtime.GOMAXPROCS(mu.Threads)
	}
	m.blockChan = mu.BlockChan

	return nil
}
