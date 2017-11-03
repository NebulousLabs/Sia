// The explorer module provides a glimpse into what the Sia network
// currently looks like.
package explorer

import (
	"errors"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	siasync "github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
	"sync"
)

const (
	// hashrateEstimationBlocks is the number of blocks that are used to
	// estimate the current hashrate.
	hashrateEstimationBlocks = 200 // 33 hours
)

var (
	errNilCS    = errors.New("explorer cannot use a nil consensus set")
	errNilTpool = errors.New("explorer cannot use a nil transaction pool")
)

type (
	// fileContractHistory stores the original file contract and the chain of
	// revisions that have affected a file contract through the life of the
	// blockchain.
	fileContractHistory struct {
		Contract     types.FileContract
		Revisions    []types.FileContractRevision
		StorageProof types.StorageProof
	}

	// blockFacts contains a set of facts about the consensus set related to a
	// certain block. The explorer needs some additional information in the
	// history so that it can calculate certain values, which is one of the
	// reasons that the explorer uses a separate struct instead of
	// modules.BlockFacts.
	blockFacts struct {
		modules.BlockFacts

		Timestamp types.Timestamp
	}

	// An Explorer contains a more comprehensive view of the blockchain,
	// including various statistics and metrics.
	Explorer struct {
		cs                               modules.ConsensusSet
		db                               *persist.BoltDatabase
		tpool                            modules.TransactionPool
		persistDir                       string
		unconfirmedSets                  map[modules.TransactionSetID][]types.TransactionID
		unconfirmedProcessedTransactions []modules.ProcessedTransaction
		mu                               sync.RWMutex
		tg                               siasync.ThreadGroup
	}
)

// New creates the internal data structures, and subscribes to
// consensus for changes to the blockchain
func New(cs modules.ConsensusSet, tpool modules.TransactionPool, persistDir string) (*Explorer, error) {
	// Check that input modules are non-nil
	if cs == nil {
		return nil, errNilCS
	}
	if tpool == nil {
		return nil, errNilTpool
	}

	// Initialize the explorer.
	e := &Explorer{
		cs:              cs,
		tpool:           tpool,
		persistDir:      persistDir,
		unconfirmedSets: make(map[modules.TransactionSetID][]types.TransactionID),
	}

	// Initialize the persistent structures, including the database.
	err := e.initPersist()
	if err != nil {
		return nil, err
	}

	// retrieve the current ConsensusChangeID
	var recentChange modules.ConsensusChangeID
	err = e.db.View(dbGetInternal(internalRecentChange, &recentChange))
	if err != nil {
		return nil, err
	}

	err = cs.ConsensusSetSubscribe(e, recentChange, nil)
	if err != nil {
		return nil, errors.New("explorer subscription failed: " + err.Error())
	}

	tpool.TransactionPoolSubscribe(e)

	return e, nil
}

// Close closes the explorer.
func (e *Explorer) Close() error {
	return e.db.Close()
}
