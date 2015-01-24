package miner

import (
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
)

type Miner struct {
	state  *consensus.State
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
func New(state *consensus.State, wallet modules.Wallet) (m *Miner, err error) {
	if state == nil {
		err = errors.New("miner cannot use a nil state")
		return
	}
	if wallet == nil {
		err = errors.New("miner cannot use a nil wallet")
		return
	}

	m = &Miner{
		state:                state,
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

// checkUpdate actually just updates the miner.
//
// TODO: checkUpdate will only update the miner if something has been sent down
// a channel.
func (m *Miner) checkUpdate() {
	select {
	case <-m.stateSubscription:
		m.state.RLock()
		m.parent = m.state.CurrentBlock().ID()
		m.transactions = m.state.TransactionPoolDump()
		m.target = m.state.CurrentTarget()
		m.earliestTimestamp = m.state.EarliestTimestamp()
		m.state.RUnlock()
	default:
		// nothing to do
	}
}
