// The explorer module provides a glimpse into what the Sia network
// currently looks like.
package explorer

import (
	"errors"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

// Basic structure to store the blockchain. Metadata may also be
// stored here in the future
type Explorer struct {
	// Factoids about file contracts.
	activeContractCost  types.Currency
	activeContractCount uint64
	activeContractSize  types.Currency
	totalContractCost   types.Currency
	totalContractCount  uint64
	totalContractSize   types.Currency
	totalRevisionVolume uint64

	// Other factoids.
	blockchainHeight       types.BlockHeight
	currentBlock           types.Block
	currencyTransferVolume types.Currency
	genesisBlockID         types.BlockID

	// startTime tracks when the explorer got turned on.
	startTime time.Time
	seenTimes []time.Time

	// Utilities.
	cs         modules.ConsensusSet
	db         *explorerDB
	persistDir string
	mu         *sync.RWMutex
}

// New creates the internal data structures, and subscribes to
// consensus for changes to the blockchain
func New(cs modules.ConsensusSet, persistDir string) (*Explorer, error) {
	// Check that input modules are non-nil
	if cs == nil {
		return nil, errors.New("explorer cannot use a nil ConsensusSet")
	}

	// Initialize the explorer.
	e := &Explorer{
		currentBlock:     cs.GenesisBlock(),
		genesisBlockID:   cs.GenesisBlock().ID(),
		seenTimes:        make([]time.Time, types.MaturityDelay+1),
		startTime:        time.Now(),

		cs:         cs,
		persistDir: persistDir,
		mu:         sync.New(modules.SafeMutexDelay, 1),
	}

	// Intialize the persistent structures, including the database.
	err := e.initPersist()
	if err != nil {
		return nil, err
	}

	cs.ConsensusSetSubscribe(e)

	return e, nil
}

// Close closes the explorer.
func (e *Explorer) Close() error {
	return e.db.CloseDatabase()
}
