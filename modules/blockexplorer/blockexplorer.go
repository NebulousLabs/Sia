package blockexplorer

import (
	"errors"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

// The blockexplorer module provides a glimpse into what the blockchain
// currently looks like.

// Basic structure to store the blockchain. Metadata may also be
// stored here in the future
type BlockExplorer struct {
	// CurBlock is the current highest block on the blockchain,
	// kept update via a subscription to consensus
	currentBlock  types.Block
	previousBlock types.Block

	// Used for caching the current blockchain height
	blockchainHeight types.BlockHeight

	// currencySent and currencySpent keep a sum of transaction volumes
	// To get a total transaction volume, programs can simply sum them up
	//
	// currencySent keeps track of ordinary transactions
	// i.e. sending siacoin to somebody else
	currencySent types.Currency

	// currencySpent holds a sum of currency spent on file contracts
	currencySpent types.Currency

	// Store the timestamps of the most recent block, so that we
	// can caluculate a moving average of the time it took to mine
	// a block
	//
	// Note: the TimestampSlice is not being used, due to this not
	// being a particularly formal setting.
	timestamps []types.Timestamp

	// Store the block sizes, so that it can be queried later
	blockSizes []uint64

	// Keep a reference to the consensus for queries
	cs modules.ConsensusSet

	// Subscriptions currently contain no data, but serve to
	// notify other modules when changes occur
	subscriptions []chan struct{}

	mu *sync.RWMutex
}

// New creates the internal data structures, and subscribes to
// consensus for changes to the blockchain
func New(cs modules.ConsensusSet) (bc *BlockExplorer, err error) {
	// Check that input modules are non-nil
	if cs == nil {
		err = errors.New("Blockchain explorer cannot use a nil ConsensusSet")
		return
	}

	// Initilize the module state
	bc = &BlockExplorer{
		currentBlock:     cs.GenesisBlock(),
		blockchainHeight: 0,
		currencySent:     types.NewCurrency64(0),
		currencySpent:    types.NewCurrency64(0),
		timestamps:       make([]types.Timestamp, 0),
		blockSizes:       make([]uint64, 0),
		cs:               cs,
		mu:               sync.New(modules.SafeMutexDelay, 1),
	}

	cs.ConsensusSetSubscribe(bc)

	return
}

// Returns the current block, as known by the current ExplorerState
func (be *BlockExplorer) CurrentBlock() types.Block {
	lockID := be.mu.RLock()
	defer be.mu.RUnlock(lockID)

	return be.currentBlock
}
