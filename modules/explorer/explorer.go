// The explorer module provides a glimpse into what the Sia network
// currently looks like.
package explorer

import (
	"errors"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	errNilCS = errors.New("explorer cannot use a nil consensus set")
)

// Basic structure to store the blockchain. Metadata may also be
// stored here in the future
type Explorer struct {
	// Factoids about file contracts.
	activeContractCost  types.Currency
	activeContractCount uint64
	activeContractSize  types.Currency
	totalContractCost   types.Currency
	totalContractSize   types.Currency
	totalRevisionVolume types.Currency

	// Transaction type counts.
	transactionCount          uint64
	siacoinInputCount         uint64
	siacoinOutputCount        uint64
	fileContractCount         uint64
	fileContractRevisionCount uint64
	storageProofCount         uint64
	siafundInputCount         uint64
	siafundOutputCount        uint64
	minerFeeCount             uint64
	arbitraryDataCount        uint64
	transactionSignatureCount uint64

	// Other factoids.
	blockchainHeight types.BlockHeight
	currentBlock     types.BlockID
	genesisBlockID   types.BlockID

	// startTime tracks when the explorer got turned on.
	startTime time.Time
	seenTimes []time.Time

	// Utilities.
	cs         modules.ConsensusSet
	persistDir string
	mu         sync.RWMutex
}

// New creates the internal data structures, and subscribes to
// consensus for changes to the blockchain
func New(cs modules.ConsensusSet, persistDir string) (*Explorer, error) {
	// Check that input modules are non-nil
	if cs == nil {
		return nil, errNilCS
	}

	// Initialize the explorer.
	e := &Explorer{
		currentBlock:   cs.GenesisBlock().ID(),
		genesisBlockID: cs.GenesisBlock().ID(),
		seenTimes:      make([]time.Time, types.MaturityDelay+1),
		startTime:      time.Now(),

		cs:         cs,
		persistDir: persistDir,
	}
	e.blockchainHeight-- // Set to -1 so the genesis block sets the height to 0.

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
	return nil
}
