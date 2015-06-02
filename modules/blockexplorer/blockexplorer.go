package blockexplorer

import (
	"errors"
	"fmt"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

// The blockexplore module provides a glimpse into what the blockchain
// currently looks like. It stores a copy of the blockchain

// Basic structure to store the blockchain. Metadata may also be
// stored here in the future
type ExplorerBlockchain struct {
	// The current state of all the blocks should be stored in
	// this slice.
	Blocks []types.Block

	mu *sync.RWMutex
}

// New creates the internal data structures, and subscribes to
// consensus for changes to the blockchain
func New(cs modules.ConsensusSet) (bc *ExplorerBlockchain, err error) {
	fmt.Println("Created an instance of the module")

	// Check that input modules are non-nil
	if cs == nil {
		err = errors.New("Blockchain explorer cannot use a nil ConsensusSet")
		return
	}

	bc = &ExplorerBlockchain{
		// TODO
		// Currently 1000 is completely arbitrary.  To change,
		// get the current number of blocks from consensus
		// somehow and initilize as that size
		Blocks: make([]types.Block, 1000),

		mu: sync.New(modules.SafeMutexDelay, 1),
	}

	cs.ConsensusSetSubscribe(bc)

	return
}
