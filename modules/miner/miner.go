package miner

import (
	"errors"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	iterationsPerAttempt = 32 * 1024
)

type Miner struct {
	cs     modules.ConsensusSet
	tpool  modules.TransactionPool
	wallet modules.Wallet

	// Block variables - helps the miner construct the next block.
	parent            types.BlockID
	height            types.BlockHeight
	transactions      []types.Transaction
	target            types.Target
	earliestTimestamp types.Timestamp
	address           types.UnlockHash

	startTime   time.Time
	attempts    uint64
	hashRate    int64
	blocksFound []types.BlockID

	threads        int // how many threads the miner uses, shouldn't ever be 0.
	desiredThreads int // 0 if not mining.
	runningThreads int

	subscribers []chan struct{}

	mu sync.Mutex
}

// New returns a ready-to-go miner that is not mining.
func New(cs modules.ConsensusSet, tpool modules.TransactionPool, w modules.Wallet) (m *Miner, err error) {
	// Create the miner and its dependencies.
	if cs == nil {
		err = errors.New("miner cannot use a nil state")
		return
	}
	if tpool == nil {
		err = errors.New("miner cannot use a nil transaction pool")
		return
	}
	if w == nil {
		err = errors.New("miner cannot use a nil wallet")
		return
	}
	currentBlock := cs.CurrentBlock().ID()
	currentTarget, exists1 := cs.ChildTarget(currentBlock)
	earliestTimestamp, exists2 := cs.EarliestChildTimestamp(currentBlock)
	if build.DEBUG {
		if !exists1 {
			panic("could not get child target")
		}
		if !exists2 {
			panic("could not get child earliest timestamp")
		}
	}
	m = &Miner{
		cs:     cs,
		tpool:  tpool,
		wallet: w,

		parent:            currentBlock,
		target:            currentTarget,
		earliestTimestamp: earliestTimestamp,

		threads: 1,
	}

	// Get an address for the miner payout.
	addr, _, err := m.wallet.CoinAddress()
	if err != nil {
		return
	}
	m.address = addr

	// Subscribe to the transaction pool to get transactions to put in blocks.
	m.tpool.TransactionPoolSubscribe(m)

	return
}

// SetThreads establishes how many threads the miner will use when mining.
func (m *Miner) SetThreads(threads int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if threads == 0 {
		return errors.New("cannot have a miner with 0 threads.")
	}
	m.threads = threads
	m.attempts = 0
	m.startTime = time.Now()

	return nil
}

// StartMining spawns a bunch of mining threads which will mine until stop is
// called.
func (m *Miner) StartMining() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Increase the number of threads to m.desiredThreads.
	m.desiredThreads = m.threads
	for i := m.runningThreads; i < m.desiredThreads; i++ {
		go m.threadedMine()
	}

	return nil
}

// StopMining sets desiredThreads to 0, a value which is polled by mining
// threads. When set to 0, the mining threads will all cease mining.
func (m *Miner) StopMining() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Set desiredThreads to 0. The miners will shut down automatically.
	m.desiredThreads = 0
	m.hashRate = 0
	return nil
}
