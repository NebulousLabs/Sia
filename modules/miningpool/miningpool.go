// Package pool is an implementation of the pool module, and is responsible for
// creating a mining pool, accepting incoming potential block solutions and
// rewarding the submitters proportionally for their shares.
package pool

// TODO: everything

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	siasync "github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// Names of the various persistent files in the pool.
	dbFilename   = modules.PoolDir + ".db"
	logFile      = modules.PoolDir + ".log"
	settingsFile = modules.PoolDir + ".json"
)

var (
	// dbMetadata is a header that gets put into the database to identify a
	// version and indicate that the database holds pool information.
	dbMetadata = persist.Metadata{
		Header:  "Sia Pool DB",
		Version: "0.0.1",
	}

	// persistMetadata is the header that gets written to the persist file, and is
	// used to recognize other persist files.
	persistMetadata = persist.Metadata{
		Header:  "Sia Pool",
		Version: "0.0.1",
	}

	// errPoolClosed gets returned when a call is rejected due to the pool
	// having been closed.
	errPoolClosed = errors.New("call is disabled because the pool is closed")

	// Nil dependency errors.
	errNilCS     = errors.New("pool cannot use a nil consensus state")
	errNilTpool  = errors.New("pool cannot use a nil transaction pool")
	errNilWallet = errors.New("pool cannot use a nil wallet")

	miningpool bool  // indicates if the mining pool is actually running
	hashRate   int64 // indicates hashes per second

)

// A Pool contains all the fields necessary for storing status for clients and
// performing the evaluation and rewarding on submitted shares
type Pool struct {
	// RPC Metrics - atomic variables need to be placed at the top to preserve
	// compatibility with 32bit systems. These values are not persistent.
	atomicDownloadCalls       uint64
	atomicErroredCalls        uint64
	atomicFormContractCalls   uint64
	atomicRenewCalls          uint64
	atomicReviseCalls         uint64
	atomicRecentRevisionCalls uint64
	atomicSettingsCalls       uint64
	atomicUnrecognizedCalls   uint64

	// Error management. There are a few different types of errors returned by
	// the pool. These errors intentionally not persistent, so that the logging
	// limits of each error type will be reset each time the pool is reset.
	// These values are not persistent.
	atomicCommunicationErrors uint64
	atomicConnectionErrors    uint64
	atomicConsensusErrors     uint64
	atomicInternalErrors      uint64
	atomicNormalErrors        uint64

	// Dependencies.
	cs     modules.ConsensusSet
	tpool  modules.TransactionPool
	wallet modules.Wallet
	dependencies
	modules.StorageManager

	// Pool ACID fields - these fields need to be updated in serial, ACID
	// transactions.
	announced         bool
	announceConfirmed bool
	blockHeight       types.BlockHeight
	publicKey         types.SiaPublicKey
	secretKey         crypto.SecretKey
	recentChange      modules.ConsensusChangeID
	unlockHash        types.UnlockHash // A wallet address that can receive coins.

	// HPoolost transient fields - these fields are either determined at startup or
	// otherwise are not critical to always be correct.
	autoAddress          modules.NetAddress // Determined using automatic tooling in network.go
	miningMetrics        modules.PoolMiningMetrics
	settings             modules.PoolInternalSettings
	revisionNumber       uint64
	workingStatus        modules.PoolWorkingStatus
	connectabilityStatus modules.PoolConnectabilityStatus

	// Utilities.
	db         *persist.BoltDatabase
	listener   net.Listener
	log        *persist.Logger
	mu         sync.RWMutex
	persistDir string
	port       string
	tg         siasync.ThreadGroup
}

// checkUnlockHash will check that the pool has an unlock hash. If the pool
// does not have an unlock hash, an attempt will be made to get an unlock hash
// from the wallet. That may fail due to the wallet being locked, in which case
// an error is returned.
func (p *Pool) checkUnlockHash() error {
	if p.unlockHash == (types.UnlockHash{}) {
		uc, err := p.wallet.NextAddress()
		if err != nil {
			return err
		}

		// Set the unlock hash and save the pool. Saving is important, because
		// the pool will be using this unlock hash to establish identity, and
		// losing it will mean silently losing part of the pool identity.
		p.unlockHash = uc.UnlockHash()
		err = p.saveSync()
		if err != nil {
			return err
		}
	}
	return nil
}

// newPool returns an initialized Pool, taking a set of dependencies as input.
// By making the dependencies an argument of the 'new' call, the pool can be
// mocked such that the dependencies can return unexpected errors or unique
// behaviors during testing, enabling easier testing of the failure modes of
// the Pool.
func newPool(dependencies dependencies, cs modules.ConsensusSet, tpool modules.TransactionPool, wallet modules.Wallet, listenerAddress string, persistDir string) (*Pool, error) {
	// Check that all the dependencies were provided.
	if cs == nil {
		return nil, errNilCS
	}
	if tpool == nil {
		return nil, errNilTpool
	}
	if wallet == nil {
		return nil, errNilWallet
	}

	// Create the pool object.
	p := &Pool{
		cs:           cs,
		tpool:        tpool,
		wallet:       wallet,
		dependencies: dependencies,

		persistDir: persistDir,
	}
	// TODO: Look at errors.go in modules/host directory for hints
	// Call stop in the event of a partial startup.
	var err error
	// defer func() {
	// 	if err != nil {
	// 		err = composeErrors(p.tg.Stop(), err)
	// 	}
	// }()

	// Create the perist directory if it does not yet exist.
	err = dependencies.mkdirAll(p.persistDir, 0700)
	if err != nil {
		return nil, err
	}

	// Initialize the logger, and set up the stop call that will close the
	// logger.
	p.log, err = dependencies.newLogger(filepath.Join(p.persistDir, logFile))
	if err != nil {
		return nil, err
	}
	p.tg.AfterStop(func() {
		err = p.log.Close()
		if err != nil {
			// State of the logger is uncertain, a Println will have to
			// suffice.
			fmt.Println("Error when closing the logger:", err)
		}
	})

	// Load the prior persistence structures, and configure the pool to save
	// before shutting down.
	err = p.load()
	if err != nil {
		return nil, err
	}
	p.tg.AfterStop(func() {
		err = p.saveSync()
		if err != nil {
			p.log.Println("Could not save pool upon shutdown:", err)
		}
	})

	// TODO:  We need this to listen for the stratum data - look at network.go in host directory for hints
	// Initialize the networking.
	// err = p.initNetworking(listenerAddress)
	// if err != nil {
	// 	p.log.Println("Could not initialize pool networking:", err)
	// 	return nil, err
	// }
	return p, nil
}

// New returns an initialized Pool.
func New(cs modules.ConsensusSet, tpool modules.TransactionPool, wallet modules.Wallet, address string, persistDir string) (*Pool, error) {
	return newPool(productionDependencies{}, cs, tpool, wallet, address, persistDir)
}

// Close shuts down the pool.
func (p *Pool) Close() error {
	return p.tg.Stop()
}

// StartPool starts the pool running
func (p *Pool) StartPool() {
	miningpool = true
}

// StopPool stops the pool running
func (p *Pool) StopPool() {
	miningpool = false
}

// GetRunning returns the running (or not) status of the pool
func (p *Pool) GetRunning() bool {
	return miningpool
}

// WorkingStatus returns the working state of the pool, where working is
// defined as having received more than workingStatusThreshold settings calls
// over the period of workingStatusFrequency.
func (p *Pool) WorkingStatus() modules.PoolWorkingStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.workingStatus
}

// ConnectabilityStatus returns the connectability state of the pool, whether
// the pool can connect to itself on its configured netaddress.
func (p *Pool) ConnectabilityStatus() modules.PoolConnectabilityStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.connectabilityStatus
}

// MiningMetrics returns information about the financial commitments,
// rewards, and activities of the pool.
func (p *Pool) MiningMetrics() modules.PoolMiningMetrics {
	p.mu.RLock()
	defer p.mu.RUnlock()
	err := p.tg.Add()
	if err != nil {
		build.Critical("Call to MiningMetrics after close")
	}
	defer p.tg.Done()
	return p.miningMetrics
}

// SetInternalSettings updates the pool's internal PoolInternalSettings object.
func (p *Pool) SetInternalSettings(settings modules.PoolInternalSettings) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	err := p.tg.Add()
	if err != nil {
		return err
	}
	defer p.tg.Done()

	// The pool should not be open for business if it does not have an
	// unlock hash.
	if settings.AcceptingShares {
		err := p.checkUnlockHash()
		if err != nil {
			return errors.New("internal settings not updated, no unlock hash: " + err.Error())
		}
	}

	p.settings = settings
	p.revisionNumber++

	err = p.saveSync()
	if err != nil {
		return errors.New("internal settings updated, but failed saving to disk: " + err.Error())
	}
	return nil
}

// InternalSettings returns the settings of a pool.
func (p *Pool) InternalSettings() modules.PoolInternalSettings {
	p.mu.RLock()
	defer p.mu.RUnlock()
	err := p.tg.Add()
	if err != nil {
		return modules.PoolInternalSettings{}
	}
	defer p.tg.Done()
	return p.settings
}
