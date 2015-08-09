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
	// that are remembered. Additionally, 'headerForWork' will only poll for a
	// new block every 'headerForWorkMemory / blockForWorkMemory' times it is
	// called. This reduces the amount of memory used, but comes at the cost of
	// not always having the most recent transactions
	headerForWorkMemory = 1000

	// blockForWorkMemory is the maximum number of blocks the miner will store
	// Blocks take up to 2 megabytes of memory, so it is important to keep a cap
	blockForWorkMemory = 20

	// secondsBetweenBlocks is the maximum amount of time the block manager will
	// go between generating new blocks. If the miner is not polling more than
	// headerForWorkMemory / blockForWorkMemory blocks every secondsBetweenBlocks
	// then the block manager will create new blocks in order to keep the miner
	// mining on the most recent block, but this will come at the cost of preventing
	// the block manger from storing as many headers
	secondsBetweenBlocks = 30
)

// Miner struct contains all variables the miner needs
// in order to create and submit blocks.
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

	// A list of blocks that have been submitted through SubmitBlock.
	blocksFound []types.BlockID

	// BlockManager variables. The BlockManager passes out and receives unique
	// block headers on each call, these variables help to map the received
	// block header to the original block. The headers are passed instead of
	// the block because a full block is 2mB and is a lot to send over http.
	// In order to store multiple headers per block, some headers map to an
	// identical address, but each header maps to a unique arbData, which can
	// be used to construct a unique block
	// lastBlock stores the Time the last block was requested.
	blockMem    map[types.BlockHeader]*types.Block
	arbDataMem  map[types.BlockHeader][]byte
	headerMem   []types.BlockHeader
	lastBlock   time.Time
	memProgress int

	// PoolManager variables. This is where addresses and contracts negotiated
	// with the pool are stored as well as constants like the percentage of a
	// block's subsidy that goes to the miner.
	// The poolTransaction is the transaction in the payment channel whose
	// output is the miner's wallet. The miner may broadcast this transaction
	// to the network at any time in order to receive payment from the pool.
	// poolHeaderMap is a map from headers of blocks that payout to the pool to
	// headers of blocks with normal payouts. This allows the poolmanager to
	// use the blockmanager and only have to worry about changing payouts.
	poolNetAddr       modules.NetAddress
	minerPercentCut   uint8
	targetMultiple    uint32
	poolPayoutAddress types.UnlockHash
	poolTransaction   types.Transaction
	poolHeaderMap     map[types.BlockHeader]types.BlockHeader

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

		blockMem:   make(map[types.BlockHeader]*types.Block),
		arbDataMem: make(map[types.BlockHeader][]byte),
		headerMem:  make([]types.BlockHeader, headerForWorkMemory),

		poolHeaderMap: make(map[types.BlockHeader]types.BlockHeader),

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
