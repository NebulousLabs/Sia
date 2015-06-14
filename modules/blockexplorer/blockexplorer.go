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
	currentBlock types.Block

	// Stored to differentiate the special case of the genesis
	// block when recieving updates from consensus, to avoid
	// constant queries to consensus.
	genesisBlockID types.BlockID

	// Used for caching the current blockchain height
	blockchainHeight types.BlockHeight

	// currencySent keeps track of how much currency has been
	// i.e. sending siacoin to somebody else
	currencySent types.Currency

	// activeContracts and totalContracts hold the current number
	// of file contracts now in effect and that ever have been,
	// respectively
	activeContracts uint64
	totalContracts  uint64

	// activeContracts and totalContracts hold the current sum
	// cost of the file contracts now in effect and that ever have
	// been, respectively
	activeContractCost types.Currency
	totalContractCost  types.Currency

	// Stores a few data points for each block:
	// Timestamp, target and size
	blockSummaries []modules.ExplorerBlockData

	// Keep a reference to the consensus for queries
	cs modules.ConsensusSet

	// Subscriptions currently contain no data, but serve to
	// notify other modules when changes occur
	subscriptions []chan struct{}

	mu *sync.RWMutex
}

// New creates the internal data structures, and subscribes to
// consensus for changes to the blockchain
func New(cs modules.ConsensusSet) (be *BlockExplorer, err error) {
	// Check that input modules are non-nil
	if cs == nil {
		err = errors.New("Blockchain explorer cannot use a nil ConsensusSet")
		return
	}

	// Initilize the module state
	be = &BlockExplorer{
		currentBlock:       cs.GenesisBlock(),
		genesisBlockID:     cs.GenesisBlock().ID(),
		blockchainHeight:   0,
		currencySent:       types.NewCurrency64(0),
		activeContractCost: types.NewCurrency64(0),
		totalContractCost:  types.NewCurrency64(0),
		cs:                 cs,
		mu:                 sync.New(modules.SafeMutexDelay, 1),
	}

	cs.ConsensusSetSubscribe(be)

	return
}
