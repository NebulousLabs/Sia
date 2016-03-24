// The explorer module provides a glimpse into what the Sia network
// currently looks like.
package explorer

import (
	"errors"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
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
	// certain block. The explorer needs some additional information in the
	// history so that it can calculate certain values, which is one of the
	// reasons that the explorer uses a separate struct instead of
	// modules.BlockFacts.
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

	// An Explorer contains a more comprehensive view of the blockchain,
	// including various statistics and metrics.
	Explorer struct {
		cs         modules.ConsensusSet
		db         *persist.BoltDatabase
		persistDir string
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
		cs:         cs,
		persistDir: persistDir,
	}

	// Intialize the persistent structures, including the database.
	err := e.initPersist()
	if err != nil {
		return nil, err
	}

	err = cs.ConsensusSetSubscribe(e, modules.ConsensusChangeBeginning)
	if err != nil {
		return nil, errors.New("explorer subscription failed: " + err.Error())
	}

	return e, nil
}

// Close closes the explorer.
func (e *Explorer) Close() error {
	return e.db.Close()
}
