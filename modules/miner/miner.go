package miner

import (
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
)

type Miner struct {
	state   *consensus.State
	gateway modules.Gateway
	tpool   modules.TransactionPool
	wallet  modules.Wallet

	// Block variables - helps the miner construct the next block.
	parent            consensus.BlockID
	transactions      []consensus.Transaction
	target            consensus.Target
	earliestTimestamp consensus.Timestamp
	address           consensus.UnlockHash

	threads              int // how many threads the miner uses, shouldn't ever be 0.
	desiredThreads       int // 0 if not mining.
	runningThreads       int
	iterationsPerAttempt uint64

	stateSubscription chan struct{}
	tpoolSubscription chan struct{}

	mu sync.RWMutex
}

// New returns a ready-to-go miner that is not mining.
func New(s *consensus.State, g modules.Gateway, tpool modules.TransactionPool, w modules.Wallet) (m *Miner, err error) {
	if s == nil {
		err = errors.New("miner cannot use a nil state")
		return
	}
	if g == nil {
		err = errors.New("miner cannot use a nil gateway")
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

	m = &Miner{
		state:                s,
		gateway:              g,
		tpool:                tpool,
		wallet:               w,
		threads:              1,
		iterationsPerAttempt: 256 * 1024,
	}

	addr, _, err := m.wallet.CoinAddress()
	if err != nil {
		return
	}
	m.address = addr

	// Update the miner.
	m.update()

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

	return nil
}

// Grabs the set of
func (m *Miner) updateTransactionSet() {
	tset, err := m.tpool.TransactionSet()
	if err != nil {
		tset = nil
	}
	m.transactions = tset
}

func (m *Miner) updateBlockInfo() {
	m.parent = m.state.CurrentBlock().ID()
	m.target = m.state.CurrentTarget()
	m.earliestTimestamp = m.state.EarliestTimestamp()
}

// update will update the mining variables to match the most recent changes in
// the blockchain and the transaction pool.
//
// Previously, these changes were only called if the state or transaction pool
// had actually changed, but this greatly increased the complexity of the code,
// and I'm not even sure it made things run faster.
func (m *Miner) update() {
	m.state.RLock()
	defer m.state.RUnlock()
	m.updateTransactionSet()
	m.updateBlockInfo()
}
