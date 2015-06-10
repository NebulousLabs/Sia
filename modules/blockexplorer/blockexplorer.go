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

	// Used for caching the current blockchain height
	blockchainHeight types.BlockHeight

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
