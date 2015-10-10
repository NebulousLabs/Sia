package miner

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// headerForWorkMemory is the number of previous calls to 'headerForWork'
	// that are remembered. Additionally, 'headerForWork' will only poll for a
	// new block every 'headerForWorkMemory / blockForWorkMemory' times it is
	// called. This reduces the amount of memory used, but comes at the cost of
	// not always having the most recent transactions. Headers take around 200
	// bytes of memory.
	headerForWorkMemory = 10000

	// blockForWorkMemory is the maximum number of blocks the miner will store
	// Blocks take up to 2 megabytes of memory.
	blockForWorkMemory = 50

	// secondsBetweenBlocks is the maximum amount of time the block manager will
	// go between generating new blocks. If the miner is not polling more than
	// headerForWorkMemory / blockForWorkMemory blocks every secondsBetweenBlocks
	// then the block manager will create new blocks in order to keep the miner
	// mining on the most recent block, but this will come at the cost of preventing
	// the block manger from storing as many headers
	secondsBetweenBlocks = 60
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

	// Miner data.
	height        types.BlockHeight // Current consensus height.
	target        types.Target      // Target of the child of the current block.
	address       types.UnlockHash  // An address which should receive miner payouts.
	blocksFound   []types.BlockID   // A list of blocks that have been found by the miner.
	unsolvedBlock types.Block       // A block containing the most recent parent and transactions.

	// BlockManager variables. Becaues blocks are large, one block is used to
	// make many headers which can be used by miners. Headers include an
	// arbitrary data transaction (appended to the block) to make the merkle
	// roots unique (preventing miners from doing redundant work). Every N
	// requests or M seconds, a new block is used to create headers.
	//
	// Only 'blocksForWorkMemory' blocks are kept in memory at a time, which
	// keeps ram usage reasonable. Miners may request many headers in parallel,
	// and thus may be working on different blocks. When they submit the solved
	// header to the block manager, the rest of the block needs to be found in
	// a lookup.
	blockMem       map[types.BlockHeader]*types.Block // Mappings from headers to the blocks they are derived from.
	arbDataMem     map[types.BlockHeader][]byte       // Mappings from the headers to their unique arb data txns.
	headerMem      []types.BlockHeader                // A circular list of headers that have been given out from the api recently.
	sourceBlockAge time.Time                          // How long headers have been using the same block (different from 'recent block').
	memProgress    int                                // The index of the most recent header used in headerMem.

	// CPUMiner variables.
	miningOn bool  // indicates if the miner is supposed to be running
	mining   bool  // indicates if the miner is actually running
	hashRate int64 // indicates hashes per second

	// Utils
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

	// Assemble the miner. The miner is assembled without an address because
	// the wallet is likely not unlocked yet. The miner will grab an address
	// after the miner is unlocked (this must be coded manually for each
	// function that potentially requires the miner to have an address.
	m := &Miner{
		cs:     cs,
		tpool:  tpool,
		wallet: w,

		blockMem:   make(map[types.BlockHeader]*types.Block),
		arbDataMem: make(map[types.BlockHeader][]byte),
		headerMem:  make([]types.BlockHeader, headerForWorkMemory),

		persistDir: persistDir,
	}
	m.height--

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
