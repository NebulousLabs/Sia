package miner

import (
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
)

type Miner struct {
	state  *consensus.State
	tpool  modules.TransactionPool
	wallet modules.Wallet

	// Block variables - helps the miner construct the next block.
	parent            consensus.BlockID
	transactions      []consensus.Transaction
	target            consensus.Target
	earliestTimestamp consensus.Timestamp
	address           consensus.CoinAddress

	threads              int // how many threads the miner uses, shouldn't ever be 0.
	desiredThreads       int // 0 if not mining.
	runningThreads       int
	iterationsPerAttempt uint64

	stateSubscription chan struct{}

	mu sync.RWMutex
}

// New returns a ready-to-go miner that is not mining.
func New(state *consensus.State, tpool modules.TransactionPool, wallet modules.Wallet) (m *Miner, err error) {
	if state == nil {
		err = errors.New("miner cannot use a nil state")
		return
	}
	if tpool == nil {
		err = errors.New("miner cannot use a nil transaction pool")
		return
	}
	if wallet == nil {
		err = errors.New("miner cannot use a nil wallet")
		return
	}

	m = &Miner{
		state:                state,
		tpool:                tpool,
		wallet:               wallet,
		threads:              1,
		iterationsPerAttempt: 256 * 1024,
	}

	// Subscribe to the state and get a mining address.
	m.stateSubscription = state.Subscribe()
	addr, _, err := m.wallet.CoinAddress()
	if err != nil {
		return
	}
	m.address = addr

	// Fool the miner into grabbing the first update.
	m.stateSubscription <- struct{}{}
	m.checkUpdate()

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

// checkUpdate will update the miner if an update has been posted by the state,
// otherwise it will do nothing.
func (m *Miner) checkUpdate() {
	select {
	case <-m.stateSubscription:
		m.state.RLock()

		// Get the transaction set from the transaction pool, using a blank set
		// if there's an error.
		tset, err := m.tpool.TransactionSet()
		if err != nil {
			tset = nil
		}

		// Update the mining variables.
		m.parent = m.state.CurrentBlock().ID()
		m.transactions = tset
		m.target = m.state.CurrentTarget()
		m.earliestTimestamp = m.state.EarliestTimestamp()

		m.state.RUnlock()
	default:
		// nothing to do
	}
}
