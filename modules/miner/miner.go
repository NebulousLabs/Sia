package miner

import (
	"errors"
	"log"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/sync"
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

	// A list of blocks that have been through SubmitBlock.
	blocksFound []types.BlockID

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

	persistDir string
	log        *log.Logger
	mu         *sync.RWMutex
}

// New returns a ready-to-go miner that is not mining.
func New(cs modules.ConsensusSet, tpool modules.TransactionPool, w modules.Wallet, persistDir string) (*Miner, error) {
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
	currentBlock := cs.GenesisBlock().ID()
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

		persistDir: persistDir,
		mu:         sync.New(modules.SafeMutexDelay, 1),
	}
	err = m.initPersist()
	if err != nil {
		return nil, err
	}
	m.tpool.TransactionPoolSubscribe(m)
	return m, nil
}

// BlocksMined returns the number of good blocks and stale blocks that have
// been mined by the miner.
func (m *Miner) BlocksMined() (goodBlocks, staleBlocks int) {
	lockID := m.mu.Lock()
	defer m.mu.Unlock(lockID)

	for _, blockID := range m.blocksFound {
		if m.cs.InCurrentPath(blockID) {
			goodBlocks++
		} else {
			staleBlocks++
		}
	}
	return
}
