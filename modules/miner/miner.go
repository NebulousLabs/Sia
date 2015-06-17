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
	iterationsPerAttempt = 16 * 1024

	// headerForWorkMemory is the number of previous calls to 'headerForWork'
	// that are remembered.
	headerForWorkMemory = 50
)

type Miner struct {
	// Module dependencies.
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

	// A list of the blocks that the miner has found.
	blocksFound []types.BlockID

	// Memory variables - used in headerforwork. blockMem maps a header to the
	// block that it is associated with. headerMem is a slice of headers used
	// to remember which headers are the N most recent headers to be requested
	// by external miners. Only the N most recent headers are kept in blockMem.

	// BlockManager variables. The BlockManager passes out and receives unique
	// block headers on each call, these variables help to map the received
	// block header to the original block. The headers are passed instead of
	// the block because a full block is 2mb and is a lot to send over http.
	blockMem    map[types.BlockHeader]types.Block
	headerMem   []types.BlockHeader
	memProgress int

	// CPUMiner variables. startTime, attempts, and hashRate are used to
	// calculate the hashrate. When attempts reaches a certain threshold, the
	// time is compared to the startTime, and divided against the number of
	// hashes per attempt, returning an approximate hashrate.
	//
	// miningOn indicates whether the miner is supposed to be mining. 'mining'
	// indicates whether these is a thread that is actively mining. There may
	// be some lag between starting the miner and a thread actually beginning
	// to mine.
	startTime time.Time
	attempts  uint64
	hashRate  int64
	miningOn  bool
	mining    bool

	// Subscription management variables.
	subscribers []chan struct{}

	mu sync.Mutex
}

// New returns a ready-to-go miner that is not mining.
func New(cs modules.ConsensusSet, tpool modules.TransactionPool, w modules.Wallet) (*Miner, error) {
	// Create the miner and its dependencies.
	if cs == nil {
		return nil, errors.New("miner cannot use a nil state")
	}
	if tpool == nil {
		return nil, errors.New("miner cannot use a nil transaction pool")
	}
	if w == nil {
		return nil, errors.New("miner cannot use a nil wallet")
	}

	// Grab some starting block variables.
	//
	// TODO: Not all of this may be needed anymore.
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
	addr, _, err := w.CoinAddress(false) // false indicates that the address should not be visible to the user.
	if err != nil {
		return nil, err
	}

	// Assemble the miner.
	m := &Miner{
		cs:     cs,
		tpool:  tpool,
		wallet: w,

		parent:            currentBlock,
		target:            currentTarget,
		earliestTimestamp: earliestTimestamp,
		address:           addr,

		blockMem:  make(map[types.BlockHeader]types.Block),
		headerMem: make([]types.BlockHeader, headerForWorkMemory),
	}
	m.tpool.TransactionPoolSubscribe(m)
	return m, nil
}

// BlocksMined returns the number of good blocks and stale blocks that have
// been mined by the miner.
func (m *Miner) BlocksMined() (goodBlocks, staleBlocks int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, blockID := range m.blocksFound {
		if m.cs.InCurrentPath(blockID) {
			goodBlocks++
		} else {
			staleBlocks++
		}
	}
	return
}
