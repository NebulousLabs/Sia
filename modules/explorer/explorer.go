// The explorer module provides a glimpse into what the Sia network
// currently looks like.
package explorer

import (
	"errors"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	siasync "github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
	"path/filepath"
	"sync"
)

const (
	// hashrateEstimationBlocks is the number of blocks that are used to
	// estimate the current hashrate.
	hashrateEstimationBlocks = 200 // 33 hours
	// logFile is the name of the log file.
	logFile = modules.ExplorerDir + ".log"
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
		persist                          peristence
		persistMu                        sync.RWMutex
		log                              *persist.Logger
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

	// Create the logger.
	e.log, err = persist.NewFileLogger(filepath.Join(e.persistDir, logFile))
	if err != nil {
		return nil, err
	}

	err = cs.ConsensusSetSubscribe(e, e.persist.RecentChange, nil)
	if err == modules.ErrInvalidConsensusChangeID {
		// Perform a rescan of the consensus set if the change id is not found.
		// The id will only be not found if there has been desynchronization
		// between the explorer and the consensus package.
		err = e.startupRescan()
		if err != nil {
			return nil, errors.New("explorer startup failed - rescanning failed: " + err.Error())
		}
	} else if err != nil {
		return nil, errors.New("explorer subscription failed: " + err.Error())
	}
	tpool.TransactionPoolSubscribe(e)

	return e, nil
}

func (e *Explorer) startupRescan() error {
	err := func() error {
		e.mu.Lock()
		defer e.mu.Unlock()

		e.persist.RecentChange = modules.ConsensusChangeBeginning
		e.persist.Height = 0
		e.persist.Target = types.Target{}
		return e.saveSync()
	}()
	if err != nil {
		return err
	}

	// Subscribe to the consensus set. This is a blocking call that will not
	// return until the explorer has fully caught up to the current block.
	err = e.cs.ConsensusSetSubscribe(e, modules.ConsensusChangeBeginning, e.tg.StopChan())
	if err != nil {
		return err
	}
	e.tg.OnStop(func() {
		e.cs.Unsubscribe(e)
	})
	return nil
}

// Close closes the explorer.
func (e *Explorer) Close() error {
	return e.db.Close()
}
