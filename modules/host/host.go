// Package host is an implementation of the host module, and is responsible for
// storing data and advertising available storage to the network.
package host

// TODO: There may need to be limitations on the window start.

// TODO: Host pricing tools.

// TODO: The host should keep an awareness of its own IP and the addresses that
// it has announced at, and should re-announce if the ip address changes or if
// the host suddenly finds itself to be unreachable.

// TODO: The host should somehow keep track of renters that make use of it,
// perhaps through public keys or something, that allows the host to know which
// renters can be safely allocated a greater number of collateral coins.
//
// Renters, especially new renters, are going to need some mechanic to ramp
// with hosts. The answer may be that new renters go through multiple
// iterations of file contracts.
//
// Adding some sort of proof-of-burn to the renter may be sufficient. If the
// renter is burning 1% coins compared to what the host is locking away (for
// new relationships), then the host can know that the renter has made
// sacrifices in excess of just locking away a proportional amount of coins.
// The renter will outright lose the coins, while the host will get the coins
// back after some time has passed.

// TODO: host_test.go has commented out tests.

// TODO: network_test.go has commented out tests.

// TODO: persist_test.go has commented out tests.

// TODO: update_test.go has commented out tests.

import (
	"crypto/rand"
	"errors"
	"net"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
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
	netAddress     modules.NetAddress
	publicKey      types.SiaPublicKey
	revisionNumber uint64
	secretKey      crypto.SecretKey
	sectorSalt     crypto.Hash
	unlockHash     types.UnlockHash

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
	storageFolders           []*storageFolder

	// Financial Metrics
	downloadBandwidthRevenue         types.Currency
	lockedStorageCollateral          types.Currency
	lostStorageCollateral            types.Currency
	lostStorageRevenue               types.Currency
	potentialStorageRevenue          types.Currency
	storageRevenue                   types.Currency
	transactionFeeExpenses           types.Currency // Amount spent on transaction fees total.
	subsidizedTransactionFeeExpenses types.Currency // Amount spent on transaction fees that the renters paid for.
	uploadBandwidthRevenue           types.Currency

	// Utilities.
	db         *persist.BoltDatabase
	listener   net.Listener
	log        *persist.Logger
	mu         sync.RWMutex
	persistDir string
	settings   modules.HostInternalSettings

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

// establishDefaults configures the default settings for the host, overwriting
// any existing settings.
func (h *Host) establishDefaults() error {
	// Configure the settings object.
	h.settings = modules.HostInternalSettings{
		MaxDuration: defaultMaxDuration,
		WindowSize:  defaultWindowSize,

		Collateral:                    defaultCollateral,
		MinimumContractPrice:          defaultContractPrice,
		MinimumDownloadBandwidthPrice: defaultDownloadBandwidthPrice,
		MinimumUploadBandwidthPrice:   defaultUploadBandwidthPrice,
	}

	// Generate signing key, for revising contracts.
	sk, pk, err := crypto.GenerateKeyPair()
	if err != nil {
		return err
	}
	h.secretKey = sk
	h.publicKey = types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       pk[:],
	}
	_, err = rand.Read(h.sectorSalt[:])
	if err != nil {
		return err
	}

	// Subscribe to the consensus set.
	err = h.initConsensusSubscription()
	if err != nil {
		return err
	}
	return h.save()
}

// initDB will check that the database has been initialized and if not, will
// initialize the database.
func (h *Host) initDB() error {
	return h.db.Update(func(tx *bolt.Tx) error {
		// The storage obligation bucket does not exist, which means the
		// database needs to be initialized. Create the database buckets.
		buckets := [][]byte{
			bucketActionItems,
			bucketSectorUsage,
			bucketStorageObligations,
		}
		for _, bucket := range buckets {
			_, err := tx.CreateBucketIfNotExists(bucket)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// newHost returns an initialized Host, taking a set of dependencies as input.
// By making the dependencies an argument of the 'new' call, the host can be
// mocked such that the dependencies can return unexpected errors or unique
// behaviors during testing, enabling easier testing of the failure modes of
// the Host.
func newHost(dependencies dependencies, cs modules.ConsensusSet, tpool modules.TransactionPool, wallet modules.Wallet, address string, persistDir string) (*Host, error) {
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

		lockedStorageObligations: make(map[types.FileContractID]struct{}),

		persistDir: persistDir,
	}

	// Create the perist directory if it does not yet exist.
	err := dependencies.mkdirAll(h.persistDir, 0700)
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
	err = h.initNetworking(address)
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
	err = h.clearPort(h.netAddress.Port())
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
