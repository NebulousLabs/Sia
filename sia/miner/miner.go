package miner

import (
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/sia/components"
)

// TODO: write docstring.
type Miner struct {
	// Block variables - helps the miner construct the next block.
	parent            consensus.BlockID
	transactions      []consensus.Transaction
	address           consensus.CoinAddress
	target            consensus.Target
	earliestTimestamp consensus.Timestamp

	threads              int // how many threads the miner uses, shouldn't ever be 0.
	desiredThreads       int // 0 if not mining.
	runningThreads       int
	iterationsPerAttempt uint64

	blockChan chan consensus.Block
	rwLock    sync.RWMutex
}

// TODO: write docstring.
func New(threads int) (m *Miner) {
	return &Miner{
		threads:              threads,
		iterationsPerAttempt: 256 * 1024,
	}
}

// UpdateMiner changes what block the miner is mining on. Changes include address
// and target.
func (m *Miner) UpdateMiner(mu components.MinerUpdate) error {
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

	return nil
}
