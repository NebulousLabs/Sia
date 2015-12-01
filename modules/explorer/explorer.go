// The explorer module provides a glimpse into what the Sia network
// currently looks like.
package explorer

import (
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// hashrateEstimationBlocks is the number of blocks that are used to
	// estimate the current hashrate.
	hashrateEstimationBlocks = 72 // 12 hours
)

var (
	errNilCS = errors.New("explorer cannot use a nil consensus set")
)

type (
	// fileContractHistory stores the original file contract and the chain of
	// revisions that have affected a file contract through the life of the
	// blockchain.
	fileContractHistory struct {
		contract     types.FileContract
		revisions    []types.FileContractRevision
		storageProof types.StorageProof
	}

	// blockFacts contians a set of facts about the consensus set related to a
	// certain block.
	blockFacts struct {
		// Block information.
		currentBlock      types.BlockID
		blockchainHeight  types.BlockHeight
		target            types.Target
		timestamp         types.Timestamp
		maturityTimestamp types.Timestamp
		estimatedHashrate types.Currency
		totalCoins        types.Currency

		// Transaction type counts.
		minerPayoutCount          uint64
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

		// Factoids about file contracts.
		activeContractCost  types.Currency
		activeContractCount uint64
		activeContractSize  types.Currency
		totalContractCost   types.Currency
		totalContractSize   types.Currency
		totalRevisionVolume types.Currency
	}

	// Basic structure to store the blockchain. Metadata may also be
	// stored here in the future
	Explorer struct {
		// Hash lookups.
		blocksDifficulty      types.Target // cumulative difficulty from the past hashrateEstimationDepth blocks.
		blockHashes           map[types.BlockID]types.BlockHeight
		blockTargets          map[types.BlockID]types.Target
		transactionHashes     map[types.TransactionID]types.BlockHeight
		unlockHashes          map[types.UnlockHash]map[types.TransactionID]struct{} // sometimes, 'txnID' is a block.
		siacoinOutputIDs      map[types.SiacoinOutputID]map[types.TransactionID]struct{}
		siacoinOutputs        map[types.SiacoinOutputID]types.SiacoinOutput
		fileContractIDs       map[types.FileContractID]map[types.TransactionID]struct{}
		fileContractHistories map[types.FileContractID]*fileContractHistory
		siafundOutputIDs      map[types.SiafundOutputID]map[types.TransactionID]struct{}
		siafundOutputs        map[types.SiafundOutputID]types.SiafundOutput

		// Utilities.
		cs         modules.ConsensusSet
		persistDir string
		mu         sync.RWMutex

		// Factoids about the current block.
		historicFacts []blockFacts
		blockFacts
	}
)

// New creates the internal data structures, and subscribes to
// consensus for changes to the blockchain
func New(cs modules.ConsensusSet, persistDir string) (*Explorer, error) {
	// Check that input modules are non-nil
	if cs == nil {
		return nil, errNilCS
	}

	// Initialize the explorer.
	e := &Explorer{
		blocksDifficulty:      types.RootDepth,
		blockHashes:           make(map[types.BlockID]types.BlockHeight),
		blockTargets:          make(map[types.BlockID]types.Target),
		transactionHashes:     make(map[types.TransactionID]types.BlockHeight),
		unlockHashes:          make(map[types.UnlockHash]map[types.TransactionID]struct{}),
		siacoinOutputIDs:      make(map[types.SiacoinOutputID]map[types.TransactionID]struct{}),
		siacoinOutputs:        make(map[types.SiacoinOutputID]types.SiacoinOutput),
		fileContractIDs:       make(map[types.FileContractID]map[types.TransactionID]struct{}),
		fileContractHistories: make(map[types.FileContractID]*fileContractHistory),
		siafundOutputIDs:      make(map[types.SiafundOutputID]map[types.TransactionID]struct{}),
		siafundOutputs:        make(map[types.SiafundOutputID]types.SiafundOutput),

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
