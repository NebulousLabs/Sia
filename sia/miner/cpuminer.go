package miner

import (
	"runtime"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
)

// Might want to switch to having a settings variable.
type CPUMiner struct {
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

// New takes a block channel down which it drops blocks that it mines. It also
// takes a thread count, which it uses to spin up miners on separate threads.
func New() (m *CPUMiner) {
	return &CPUMiner{
		iterationsPerAttempt: 256 * 1024,
	}
}

// SubsidyAddress returns the address that is currently being used by the miner
// while searching for blocks.
func (m *CPUMiner) SubsidyAddress() consensus.CoinAddress {
	m.Lock()
	defer m.Unlock()

	return m.address
}

// Update changes what block the miner is mining on. Changes include address
// and target.
func (m *CPUMiner) Update(mu MinerUpdate) error {
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
