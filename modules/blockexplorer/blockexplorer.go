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

	// currencySent keeps track of how much currency has been
	// i.e. sending siacoin to somebody else
	currencySent types.Currency

	// fileContracts holds the current number of file contracts
	fileContracts uint64

	// fileContractCost hold the amout of currency tied up in file
	// contracts
	fileContractCost types.Currency

	// Stores a few data points for each block:
	// Timestamp, target and size
	blocks []modules.BlockData

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
	bc = &ExplorerState{
		currentBlock:     cs.CurrentBlock(),
		blockchainHeight: 1, // genesis block
		currencySent:     types.NewCurrency64(0),
		fileContracts:    0,
		fileContractCost: types.NewCurrency64(0),
		blocks:           make([]modules.BlockData, 0),
		cs:               cs,
		mu:               sync.New(modules.SafeMutexDelay, 1),
	}

	cs.ConsensusSetSubscribe(bc)

	return
}
