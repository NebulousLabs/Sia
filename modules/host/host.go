// Package host is an implementation of the host module, and is responsible for
// participating in the storage ecosystem, turning available disk space an
// internet bandwidth into profit for the user.
package host

// TODO: automated_settings.go, a file which can be responsible for
// automatically regulating things like bandwidth price, storage price,
// contract price, etc. One of the features in consideration is that the host
// would start to steeply increase the contract price as it begins to run low
// on collateral. The host would also inform the user that there doesn't seem
// to be enough money to handle all of the file contracts, so that the user
// could make a judgement call on whether to get more.

// TODO: The host needs to somehow keep an awareness of its bandwidth limits,
// and needs to reject calls if there is not enough bandwidth available.

// TODO: The synchronization on the port forwarding is not perfect. Sometimes a
// port will be cleared before it was set (if things happen fast enough),
// because the port forwarding call is asynchronous.

// TODO: Add contract compensation from form contract to the storage obligation
// financial metrics, and to the host's tracking.

// TODO: merge the network interfaces stuff, don't forget to include the
// 'announced' variable as one of the outputs.

// TODO: check that the host is doing proper clean shudown, especially
// network.go, a couple of problems with clean shutdown in network.go.

// TODO: clean up all of the magic numbers in the host.

// TODO: host_test.go has commented out tests.

// TODO: network_test.go has commented out tests.

// TODO: persist_test.go has commented out tests.

// TODO: update_test.go has commented out tests.

import (
	"errors"
	"net"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
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
	// compatibility with 32bit systems.
	atomicDownloadCalls     uint64
	atomicErroredCalls      uint64
	atomicFormContractCalls uint64
	atomicRenewCalls        uint64
	atomicReviseCalls       uint64
	atomicSettingsCalls     uint64
	atomicUnrecognizedCalls uint64

	// Dependencies.
	cs     modules.ConsensusSet
	tpool  modules.TransactionPool
	wallet modules.Wallet
	dependencies

	// Consensus Tracking.
	blockHeight  types.BlockHeight
	recentChange modules.ConsensusChangeID

	// Host Identity
	//
	// The revision number keeps track of the current revision number on the
	// host external settingse
	//
	// The auto address is the address that is fetched automatically by the
	// host. The host will ignore the automatic address if settings.NetAddress
	// has been set by the user. If settings.NetAddress is blank, then the host
	// will track its own ip address and make an announcement on the blockchain
	// every time that the address changes.
	//
	// The announced bool indicates whether the host remembers having a
	// successful announcement with the current address.
	announced        bool
	autoAddress      modules.NetAddress
	financialMetrics modules.HostFinancialMetrics
	publicKey        types.SiaPublicKey
	revisionNumber   uint64
	secretKey        crypto.SecretKey
	settings         modules.HostInternalSettings
	unlockHash       types.UnlockHash // A wallet address that can receive coins.

	// Storage Obligation Management - different from file management in that
	// the storage obligation management is the new way of handling storage
	// obligations. Is a replacement for the contract obligation logic, but the
	// old logic is being kept for compatibility purposes.
	//
	// Storage is broken up into sectors. The sectors are distributed across a
	// set of storage folders using a strategy that tries to create even
	// distributions, but not aggressively. Uneven distributions could be
	// manufactured by an attacker given sufficent knowledge about the disk
	// layout (knowledge which should be unavailable), but a limited amount of
	// damage can be done even with this attack.
	lockedStorageObligations map[types.FileContractID]struct{} // Which storage obligations are currently being modified.
	sectorSalt               crypto.Hash
	storageFolders           []*storageFolder

	// Utilities.
	db         *persist.BoltDatabase
	listener   net.Listener
	log        *persist.Logger
	mu         sync.RWMutex
	persistDir string
	port       string

	// The resource lock is held by threaded functions for the duration of
	// their operation. Functions should grab the resource lock as a read lock
	// unless they are planning on manipulating the 'closed' variable.
	// Readlocks are used so that multiple functions can use resources
	// simultaneously, but the resources are not closed until all functions
	// accessing them have returned.
	closed       bool
	resourceLock sync.RWMutex
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
		err = h.save()
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

	// Parse the port from the address.
	_, port, err := net.SplitHostPort(listenerAddress)
	if err != nil {
		return nil, err
	}

	// Create the host object.
	h := &Host{
		cs:           cs,
		tpool:        tpool,
		wallet:       wallet,
		dependencies: dependencies,

		lockedStorageObligations: make(map[types.FileContractID]struct{}),

		persistDir: persistDir,
		port:       port,
	}

	// Create the perist directory if it does not yet exist.
	err = dependencies.mkdirAll(h.persistDir, 0700)
	if err != nil {
		return nil, err
	}

	// Initialize the logger. Logging should be initialized ASAP, because the
	// rest of the initialization makes use of the logger.
	h.log, err = dependencies.newLogger(filepath.Join(h.persistDir, logFile))
	if err != nil {
		return nil, err
	}

	// Open the database containing the host's storage obligation metadata.
	h.db, err = dependencies.openDatabase(dbMetadata, filepath.Join(h.persistDir, dbFilename))
	if err != nil {
		// An error will be returned if the database has the wrong version, but
		// as of writing there was only one version of the database and all
		// other databases would be incompatible.
		_ = h.log.Close()
		return nil, err
	}
	// After opening the database, it must be initalized. Most commonly,
	// nothing happens. But for new databases, a set of buckets must be
	// created. Intialization is also a good time to run sanity checks.
	err = h.initDB()
	if err != nil {
		_ = h.log.Close()
		_ = h.db.Close()
		return nil, err
	}

	// Load the prior persistance structures.
	err = h.load()
	if err != nil {
		_ = h.log.Close()
		_ = h.db.Close()
		return nil, err
	}

	// Get the host established on the network.
	err = h.initNetworking(listenerAddress)
	if err != nil {
		_ = h.log.Close()
		_ = h.db.Close()
		return nil, err
	}

	return h, nil
}

// New returns an initialized Host.
func New(cs modules.ConsensusSet, tpool modules.TransactionPool, wallet modules.Wallet, address string, persistDir string) (*Host, error) {
	return newHost(productionDependencies{}, cs, tpool, wallet, address, persistDir)
}

// Close shuts down the host, preparing it for garbage collection.
func (h *Host) Close() (composedError error) {
	// Unsubscribe the host from the consensus set. Call will not terminate
	// until the last consensus update has been sent to the host.
	// Unsubscription must happen before any resources are released or
	// terminated because the process consensus change function makes use of
	// those resources.
	h.cs.Unsubscribe(h)

	// Close the listener, which means incoming network connections will be
	// rejected. The listener should be closed before the host resources are
	// disabled, as incoming connections will want to use the hosts resources.
	err := h.listener.Close()
	if err != nil {
		composedError = composeErrors(composedError, err)
	}

	// Grab the resource lock and indicate that the host is closing. Concurrent
	// functions hold the resource lock until they terminate, meaning that no
	// threaded function will be running by the time the resource lock is
	// acquired.
	h.resourceLock.Lock()
	h.closed = true
	h.resourceLock.Unlock()

	// Close the bolt database.
	err = h.db.Close()
	if err != nil {
		composedError = composeErrors(composedError, err)
	}

	// Clear the port that was forwarded at startup. The port handling must
	// happen before the logger is closed, as it leaves a logging message.
	err = h.managedClearPort()
	if err != nil {
		composedError = composeErrors(composedError, err)
	}

	// Save the latest host state.
	h.mu.Lock()
	err = h.save()
	h.mu.Unlock()
	if err != nil {
		composedError = composeErrors(composedError, err)
	}

	// Close the logger. The logger should be the last thing to shut down so
	// that all other objects have access to logging while closing.
	err = h.log.Close()
	if err != nil {
		composedError = composeErrors(composedError, err)
	}
	return composedError
}

// FinancialMetrics returns information about the financial commitments,
// rewards, and activities of the host.
func (h *Host) FinancialMetrics() modules.HostFinancialMetrics {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.financialMetrics
}

// SetInternalSettings updates the host's internal HostInternalSettings object.
func (h *Host) SetInternalSettings(settings modules.HostInternalSettings) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.resourceLock.RLock()
	defer h.resourceLock.RUnlock()
	if h.closed {
		return errHostClosed
	}

	// The host should not be accepting file contracts if it does not have an
	// unlock hash.
	if settings.AcceptingContracts {
		err := h.checkUnlockHash()
		if err != nil {
			return err
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
	return h.save()
}

// InternalSettings returns the settings of a host.
func (h *Host) InternalSettings() modules.HostInternalSettings {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.settings
}
