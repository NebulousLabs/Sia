package miner

import (
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/types"
)

type Miner struct {
	state  *consensus.State
	tpool  modules.TransactionPool
	wallet modules.Wallet

	// Block variables - helps the miner construct the next block.
	parent            types.BlockID
	height            types.BlockHeight
	transactions      []types.Transaction
	target            types.Target
	earliestTimestamp types.Timestamp
	address           types.UnlockHash

	attempts int

	threads              int // how many threads the miner uses, shouldn't ever be 0.
	desiredThreads       int // 0 if not mining.
	runningThreads       int
	iterationsPerAttempt uint64

	subscribers []chan struct{}

	mu sync.Mutex
}

// New returns a ready-to-go miner that is not mining.
func New(s *consensus.State, tpool modules.TransactionPool, w modules.Wallet) (m *Miner, err error) {
	if s == nil {
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

	m = &Miner{
		state:  s,
		tpool:  tpool,
		wallet: w,

		parent:            s.CurrentBlock().ID(),
		target:            s.CurrentTarget(),
		earliestTimestamp: s.EarliestTimestamp(),

		threads:              1,
		iterationsPerAttempt: 64 * 1024,
	}

	addr, _, err := m.wallet.CoinAddress()
	if err != nil {
		return
	}
	m.address = addr

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

	return nil
}
