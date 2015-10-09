package miner

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// headerForWorkMemory is the number of previous calls to 'headerForWork'
	// that are remembered. Additionally, 'headerForWork' will only poll for a
	// new block every 'headerForWorkMemory / blockForWorkMemory' times it is
	// called. This reduces the amount of memory used, but comes at the cost of
	// not always having the most recent transactions
	headerForWorkMemory = 10000

	// blockForWorkMemory is the maximum number of blocks the miner will store
	// Blocks take up to 2 megabytes of memory, so it is important to keep a cap
	blockForWorkMemory = 50

	// secondsBetweenBlocks is the maximum amount of time the block manager will
	// go between generating new blocks. If the miner is not polling more than
	// headerForWorkMemory / blockForWorkMemory blocks every secondsBetweenBlocks
	// then the block manager will create new blocks in order to keep the miner
	// mining on the most recent block, but this will come at the cost of preventing
	// the block manger from storing as many headers
	secondsBetweenBlocks = 30
)

var (
	errNilCS     = errors.New("miner cannot use a nil consensus set")
	errNilTpool  = errors.New("miner cannot use a nil transaction pool")
	errNilWallet = errors.New("miner cannot use a nil wallet")
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

	// A list of blocks that have been submitted to the miner.
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

	// CPUMiner variables.
	miningOn   bool      // indicates if the miner is supposed to be running
	mining     bool      // indicates if the miner is actually running
	cycleStart time.Time // indicates the start time of the recent call to SolveBlock
	hashRate   int64     // indicates hashes per second

	persistDir string
	log        *log.Logger
	mu         sync.RWMutex
}

// New returns a ready-to-go miner that is not mining.
func New(cs modules.ConsensusSet, tpool modules.TransactionPool, w modules.Wallet, persistDir string) (*Miner, error) {
	// Create the miner and its dependencies.
	if cs == nil {
		return nil, errNilCS
	}
	if tpool == nil {
		return nil, errNilTpool
	}
	if w == nil {
		return nil, errNilWallet
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

	// Assemble the miner. The miner is assembled without an address because
	// the wallet is likely not unlocked yet. The miner will grab an address
	// after the miner is unlocked (this must be coded manually for each
	// function that potentially requires the miner to have an address.
	m := &Miner{
		cs:     cs,
		tpool:  tpool,
		wallet: w,

		parent:            currentBlock,
		target:            currentTarget,
		earliestTimestamp: earliestTimestamp,

		blockMem:   make(map[types.BlockHeader]*types.Block),
		arbDataMem: make(map[types.BlockHeader][]byte),
		headerMem:  make([]types.BlockHeader, headerForWorkMemory),

		persistDir: persistDir,
	}
	err := m.initPersist()
	if err != nil {
		return nil, err
	}
	m.cs.ConsensusSetDigestSubscribe(m)
	m.tpool.TransactionPoolSubscribe(m)
	return m, nil
}

// checkAddress checks that the miner has an address, fetching an address from
// the wallet if not.
func (m *Miner) checkAddress() error {
	if m.address != (types.UnlockHash{}) {
		return nil
	}
	uc, err := m.wallet.NextAddress()
	if err != nil {
		return err
	}
	m.address = uc.UnlockHash()
	return nil
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
