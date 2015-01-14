package miner

import (
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/sia/components"
)

// TODO: integrate the miner as a state listener.
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
	mu        sync.RWMutex
}

// New returns a miner that needs to be updated/initialized.
//
// TODO: Formalize components so that
func New() (m *Miner) {
	return &Miner{
		threads:              1,
		iterationsPerAttempt: 256 * 1024,
	}
}

// TODO: write docstring.
//
// TODO: contemplate giving the miner access to a read only state that it
// queries for block information, instead of needing to pass all of that
// information through the update struct.
func (m *Miner) UpdateMiner(mu components.MinerUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()

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
