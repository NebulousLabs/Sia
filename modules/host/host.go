// Package host is an implementation of the host module, and is responsible for
// participating in the storage ecosystem, turning available disk space an
// internet bandwidth into profit for the user.
package host

// TODO: what happens if the renter submits the revision early, before the
// final revision. Will the host mark the contract as complete?

// TODO: Host and renter are reporting errors where the renter is not adding
// enough fees to the file contract.

// TODO: Test the safety of the builder, it should be okay to have multiple
// builders open for up to 600 seconds, which means multiple blocks could be
// received in that time period. Should also check what happens if a prent gets
// confirmed on the blockchain before the builder is finished.

// TODO: Double check that any network connection has a finite deadline -
// handling action items properly requires that the locks held on the
// obligations eventually be released. There's also some more advanced
// implementation that needs to happen with the storage obligation locks to
// make sure that someone who wants a lock is able to get it eventually.

// TODO: Add contract compensation from form contract to the storage obligation
// financial metrics, and to the host's tracking.

// TODO: merge the network interfaces stuff, don't forget to include the
// 'announced' variable as one of the outputs.

// TODO: 'announced' doesn't tell you if the announcement made it to the
// blockchain.

// TODO: Need to make sure that the revision exchange for the renter and the
// host is being handled correctly. For the host, it's not so difficult. The
// host need only send the most recent revision every time. But, the host
// should not sign a revision unless the renter has explicitly signed such that
// the 'WholeTransaction' fields cover only the revision and that the
// signatures for the revision don't depend on anything else. The renter needs
// to verify the same when checking on a file contract revision from the host.
// If the host has submitted a file contract revision where the signatures have
// signed the whole file contract, there is an issue.

// TODO: there is a mistake in the file contract revision rpc, the host, if it
// does not have the right file contract id, should be returning an error there
// to the renter (and not just to it's calling function without informing the
// renter what's up).

// TODO: Need to make sure that the correct height is being used when adding
// sectors to the storage manager - in some places right now WindowStart is
// being used but really it's WindowEnd that should be in use.

// TODO: The host needs some way to blacklist file contracts that are being
// abusive by repeatedly getting free download batches.

// TODO: clean up all of the magic numbers in the host.

// TODO: revamp the finances for the storage obligations.

// TODO: host_test.go has commented out tests.

// TODO: network_test.go has commented out tests.

// TODO: persist_test.go has commented out tests.

// TODO: update_test.go has commented out tests.

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/host/storagemanager"
	"github.com/NebulousLabs/Sia/persist"
	siasync "github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// Names of the various persistent files in the host.
	dbFilename   = modules.HostDir + ".db"
	logFile      = modules.HostDir + ".log"
	settingsFile = modules.HostDir + ".json"
)

var (
	// dbMetadata is a header that gets put into the database to identify a
	// version and indicate that the database holds host information.
	dbMetadata = persist.Metadata{
		Header:  "Sia Host DB",
		Version: "0.5.2",
	}

	// persistMetadata is the header that gets written to the persist file, and is
	// used to recognize other persist files.
	persistMetadata = persist.Metadata{
		Header:  "Sia Host",
		Version: "0.5",
	}

	// errHostClosed gets returned when a call is rejected due to the host
	// having been closed.
	errHostClosed = errors.New("call is disabled because the host is closed")

	// Nil dependency errors.
	errNilCS     = errors.New("host cannot use a nil state")
	errNilTpool  = errors.New("host cannot use a nil transaction pool")
	errNilWallet = errors.New("host cannot use a nil wallet")
)

// A Host contains all the fields necessary for storing files for clients and
// performing the storage proofs on the received files.
type Host struct {
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
	// the host. These errors intentionally not persistent, so that the logging
	// limits of each error type will be reset each time the host is reset.
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

	// Host ACID fields - these fields need to be updated in serial, ACID
	// transactions.
	announced         bool
	announceConfirmed bool
	blockHeight       types.BlockHeight
	publicKey         types.SiaPublicKey
	secretKey         crypto.SecretKey
	recentChange      modules.ConsensusChangeID
	unlockHash        types.UnlockHash // A wallet address that can receive coins.

	// Host transient fields - these fields are either determined at startup or
	// otherwise are not critical to always be correct.
	autoAddress      modules.NetAddress // Determined using automatic tooling in network.go
	financialMetrics modules.HostFinancialMetrics
	settings         modules.HostInternalSettings
	revisionNumber   uint64

	// A map of storage obligations that are currently being modified. Locks on
	// storage obligations can be long-running, and each storage obligation can
	// be locked separately.
	lockedStorageObligations map[types.FileContractID]*siasync.TryMutex

	// Utilities.
	db         *persist.BoltDatabase
	listener   net.Listener
	log        *persist.Logger
	mu         sync.RWMutex
	persistDir string
	port       string
	tg         siasync.ThreadGroup
}

// checkUnlockHash will check that the host has an unlock hash. If the host
// does not have an unlock hash, an attempt will be made to get an unlock hash
// from the wallet. That may fail due to the wallet being locked, in which case
// an error is returned.
func (h *Host) checkUnlockHash() error {
	if h.unlockHash == (types.UnlockHash{}) {
		uc, err := h.wallet.NextAddress()
		if err != nil {
			return err
		}

		// Set the unlock hash and save the host. Saving is important, because
		// the host will be using this unlock hash to establish identity, and
		// losing it will mean silently losing part of the host identity.
		h.unlockHash = uc.UnlockHash()
		err = h.saveSync()
		if err != nil {
			return err
		}
	}
	return nil
}

// newHost returns an initialized Host, taking a set of dependencies as input.
// By making the dependencies an argument of the 'new' call, the host can be
// mocked such that the dependencies can return unexpected errors or unique
// behaviors during testing, enabling easier testing of the failure modes of
// the Host.
func newHost(dependencies dependencies, cs modules.ConsensusSet, tpool modules.TransactionPool, wallet modules.Wallet, listenerAddress string, persistDir string) (*Host, error) {
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

	// Create the host object.
	h := &Host{
		cs:           cs,
		tpool:        tpool,
		wallet:       wallet,
		dependencies: dependencies,

		lockedStorageObligations: make(map[types.FileContractID]*siasync.TryMutex),

		persistDir: persistDir,
	}

	// Call stop in the event of a partial startup.
	var err error
	defer func() {
		if err != nil {
			err = composeErrors(h.tg.Stop(), err)
		}
	}()

	// Create the perist directory if it does not yet exist.
	err = dependencies.mkdirAll(h.persistDir, 0700)
	if err != nil {
		return nil, err
	}

	// Initialize the logger, and set up the stop call that will close the
	// logger.
	h.log, err = dependencies.newLogger(filepath.Join(h.persistDir, logFile))
	if err != nil {
		return nil, err
	}
	h.tg.AfterStop(func() {
		err = h.log.Close()
		if err != nil {
			// State of the logger is uncertain, a Println will have to
			// suffice.
			fmt.Println("Error when closing the logger:", err)
		}
	})

	// Add the storage manager to the host, and set up the stop call that will
	// close the storage manager.
	h.StorageManager, err = storagemanager.New(filepath.Join(persistDir, "storagemanager"))
	if err != nil {
		h.log.Println("Could not open the storage manager:", err)
		return nil, err
	}
	h.tg.AfterStop(func() {
		err = h.StorageManager.Close()
		if err != nil {
			h.log.Println("Could not close storage manager:", err)
		}
	})

	// After opening the database, it must be initialized. Most commonly,
	// nothing happens. But for new databases, a set of buckets must be
	// created. Initialization is also a good time to run sanity checks.
	err = h.initDB()
	if err != nil {
		h.log.Println("Could not initalize database:", err)
		return nil, err
	}

	// Load the prior persistence structures, and configure the host to save
	// before shutting down.
	err = h.load()
	if err != nil {
		return nil, err
	}
	h.tg.AfterStop(func() {
		err = h.saveSync()
		if err != nil {
			h.log.Println("Could not save host upon shutdown:", err)
		}
	})

	// Initialize the networking.
	err = h.initNetworking(listenerAddress)
	if err != nil {
		h.log.Println("Could not initialize host networking:", err)
		return nil, err
	}
	return h, nil
}

// New returns an initialized Host.
func New(cs modules.ConsensusSet, tpool modules.TransactionPool, wallet modules.Wallet, address string, persistDir string) (*Host, error) {
	return newHost(productionDependencies{}, cs, tpool, wallet, address, persistDir)
}

// Close shuts down the host.
func (h *Host) Close() error {
	return h.tg.Stop()
}

// ExternalSettings returns the hosts external settings. These values cannot be
// set by the user (host is configured through InternalSettings), and are the
// values that get displayed to other hosts on the network.
func (h *Host) ExternalSettings() modules.HostExternalSettings {
	h.mu.RLock()
	defer h.mu.RUnlock()
	err := h.tg.Add()
	if err != nil {
		build.Critical("Call to ExternalSettings after close")
	}
	defer h.tg.Done()
	return h.externalSettings()
}

// FinancialMetrics returns information about the financial commitments,
// rewards, and activities of the host.
func (h *Host) FinancialMetrics() modules.HostFinancialMetrics {
	h.mu.RLock()
	defer h.mu.RUnlock()
	err := h.tg.Add()
	if err != nil {
		build.Critical("Call to FinancialMetrics after close")
	}
	defer h.tg.Done()
	return h.financialMetrics
}

// SetInternalSettings updates the host's internal HostInternalSettings object.
func (h *Host) SetInternalSettings(settings modules.HostInternalSettings) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	err := h.tg.Add()
	if err != nil {
		return err
	}
	defer h.tg.Done()

	// The host should not be accepting file contracts if it does not have an
	// unlock hash.
	if settings.AcceptingContracts {
		err := h.checkUnlockHash()
		if err != nil {
			return errors.New("internal settings not updated, no unlock hash: " + err.Error())
		}
	}

	if settings.NetAddress != "" {
		err := settings.NetAddress.IsValid()
		if err != nil {
			return errors.New("internal settings not updated, invalid NetAddress: " + err.Error())
		}
	}

	// Check if the net address for the host has changed. If it has, and it's
	// not equal to the auto address, then the host is going to need to make
	// another blockchain announcement.
	if h.settings.NetAddress != settings.NetAddress && settings.NetAddress != h.autoAddress {
		h.announced = false
	}

	h.settings = settings
	h.revisionNumber++

	err = h.saveSync()
	if err != nil {
		return errors.New("internal settings updated, but failed saving to disk: " + err.Error())
	}
	return nil
}

// InternalSettings returns the settings of a host.
func (h *Host) InternalSettings() modules.HostInternalSettings {
	h.mu.RLock()
	defer h.mu.RUnlock()
	err := h.tg.Add()
	if err != nil {
		return modules.HostInternalSettings{}
	}
	defer h.tg.Done()
	return h.settings
}
