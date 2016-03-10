package host

import (
	"errors"
	"sync/atomic"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

var (
	// errChangedRemainingStorage is returned by SetSettings if the remaining
	// storage has changed, an illegal operation.
	errChangedRemainingStorage = errors.New("cannot change the remaining storage in SetSettings")

	// errChangedTotalStorage is returned by SetSettings if the total storage
	// has changed, an illegal operation.
	errChangedTotalStorage = errors.New("cannot change the total storage in SetSettings")

	// errChangedUnlockHash is returned by SetSettings if the unlock hash has
	// changed, an illegal operation.
	errChangedUnlockHash = errors.New("cannot change the unlock hash in SetSettings")
)

// capacity returns the amount of storage still available on the machine. The
// amount can be negative if the total capacity was reduced to below the active
// capacity.
func (h *Host) capacity() (total uint64, remaining uint64, err error) {
	// This call needs to access a database to count the amount of storage in
	// use, so the resource lock must be acquired.
	h.resourceLock.RLock()
	defer h.resourceLock.RUnlock()
	if h.closed {
		return 0, 0, errHostClosed
	}

	// Total storage can be computed by summing the size of all the storage
	// folders.
	for _, sf := range h.storageFolders {
		total += sf.Size
		remaining += sf.SizeRemaining
	}
	return total, remaining, nil
}

// Capacity returns the amount of storage still available on the machine. The
// amount can be negative if the total capacity was reduced to below the active
// capacity.
func (h *Host) Capacity() (total uint64, remaining uint64, err error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.capacity()
}

// Contracts returns the number of unresolved file contracts that the host is
// responsible for.
func (h *Host) Contracts() (uint64, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var numContracts uint64
	err := h.db.View(func(tx *bolt.Tx) error {
		numContracts = uint64(tx.Bucket(bucketStorageObligations).Stats().KeyN)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return numContracts, nil
}

// NetAddress returns the address at which the host can be reached.
func (h *Host) NetAddress() modules.NetAddress {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.netAddress
}

// Revenue returns the amount of revenue that the host has lined up, as well as
// the amount of revenue that the host has successfully captured.
func (h *Host) Revenue() (unresolved, resolved, lost types.Currency) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.potentialStorageRevenue, h.storageRevenue, h.lostStorageRevenue
}

// RPCMetrics returns information about the types of rpc calls that have been
// made to the host.
func (h *Host) RPCMetrics() modules.HostRPCMetrics {
	return modules.HostRPCMetrics{
		ErrorCalls:        atomic.LoadUint64(&h.atomicErroredCalls),
		UnrecognizedCalls: atomic.LoadUint64(&h.atomicUnrecognizedCalls),
		DownloadCalls:     atomic.LoadUint64(&h.atomicDownloadCalls),
		RenewCalls:        atomic.LoadUint64(&h.atomicRenewCalls),
		ReviseCalls:       atomic.LoadUint64(&h.atomicReviseCalls),
		SettingsCalls:     atomic.LoadUint64(&h.atomicSettingsCalls),
		UploadCalls:       atomic.LoadUint64(&h.atomicUploadCalls),
	}
}

// Settings returns the settings of a host.
func (h *Host) Settings() modules.HostInternalSettings {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.settings
}
