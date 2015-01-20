package miner

import (
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/sia/components"
)

type Miner struct {
	state  *consensus.State
	wallet components.Wallet

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

	// TODO: Depricate
	blockChan chan consensus.Block

	mu sync.RWMutex
}

// New returns a ready-to-go miner that is not mining.
func New(state *consensus.State, wallet components.Wallet) (m *Miner, err error) {
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

	m.update()
	go m.threadedListen()

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

// update will readlock the state and update all of the miner's block variables
// in one atomic action.
//
// TODO: For some reason, these locks will cause a deadlock during the testing.
// Commented out for now, but we need to figure out why there's a deadlock.
// Once the state locks, it shouldn't depend on any other interaction to
// unlock, and these locks are only read-locks, which means there should be no
// interference.
//
// The deadlocking problem is especially weird because all of the internal
// functions call RLock as well. If they can get the RLock without problems,
// why can't update() do it at the same time?
//
// Also weird, inverting the comments in this function still results in
// deadlock. So for some reason just calling m.state.RLock() causes problems.
// No idea what to do about it, except maybe give up on the idea of letting
// people lock the state, and instead implementing functions in the consensus
// package that will return all of this information in one go. But that's ugly
// too.
//
// Until the miner has an atomic way to grab the 4 values, this is a race
// condition, but not one that the race library will be able to detect.
func (m *Miner) update() {
	// m.state.RLock()
	m.parent = m.state.CurrentBlock().ID()
	m.transactions = m.state.TransactionPoolDump()
	m.target = m.state.CurrentTarget()
	m.earliestTimestamp = m.state.EarliestTimestamp()
	// m.state.RUnlock()
}

// listen will continuously wait for an update notification from the state, and
// then call update() upon receiving one.
func (m *Miner) threadedListen() {
	for {
		select {
		case _ = <-m.stateSubscription:
			m.mu.Lock()
			m.update()
			m.mu.Unlock()
		}
	}
}

// TODO: depricate. This is gross but it's only here while I move everything
// over to subscription. Stuff will break if the miner isn't feeding blocks
// directly to the core instead of directly to the state.
func (m *Miner) SetBlockChan(blockChan chan consensus.Block) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blockChan = blockChan
}
