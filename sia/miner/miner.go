package miner

import (
	"errors"
	"runtime"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/sia/components"
)

// Might want to switch to having a settings variable.
type Miner struct {
	// Block variables - helps the miner construct the next block.
	parent            consensus.BlockID
	transactions      []consensus.Transaction
	address           consensus.CoinAddress
	target            consensus.Target
	earliestTimestamp consensus.Timestamp

	threads              int // how many threads the miner uses.
	desiredThreads       int // 0 if not mining.
	runningThreads       int
	iterationsPerAttempt uint64

	blockChan chan consensus.Block
	rwLock    sync.RWMutex
}

// New takes a block channel down which it drops blocks that it mines. It also
// takes a thread count, which it uses to spin up miners on separate threads.
func New() (m *Miner) {
	return &Miner{
		iterationsPerAttempt: 256 * 1024,
	}
}

// SubsidyAddress returns the address that is currently being used by the miner
// while searching for blocks.
func (m *Miner) SubsidyAddress() consensus.CoinAddress {
	m.lock()
	defer m.unlock()
	return m.address
}

// Update changes what block the miner is mining on. Changes include address
// and target.
func (m *Miner) Update(mu components.MinerUpdate) error {
	m.lock()
	defer m.unlock()

	if mu.Threads == 0 {
		return errors.New("cannot have a miner with 0 threads.")
	}

	m.parent = mu.Parent
	m.transactions = mu.Transactions
	m.target = mu.Target
	m.address = mu.Address
	m.earliestTimestamp = mu.EarliestTimestamp
	m.threads = mu.Threads
	m.blockChan = mu.BlockChan

	runtime.GOMAXPROCS(mu.Threads)

	return nil
}
