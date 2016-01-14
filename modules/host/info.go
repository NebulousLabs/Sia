package host

import (
	"sync/atomic"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// Capacity returns the amount of storage still available on the machine. The
// amount can be negative if the total capacity was reduced to below the active
// capacity.
func (h *Host) Capacity() int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.spaceRemaining
}

// Contracts returns the number of unresolved file contracts that the host is
// responsible for.
func (h *Host) Contracts() uint64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return uint64(len(h.obligationsByID))
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
	return h.anticipatedRevenue, h.revenue, h.lostRevenue
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

// SetSettings updates the host's internal HostSettings object.
func (h *Host) SetSettings(settings modules.HostSettings) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.resourceLock.RLock()
	defer h.resourceLock.RUnlock()
	if h.closed {
		return errHostClosed
	}

	// Check that the unlock hash was not changed.
	if settings.UnlockHash != h.settings.UnlockHash {
		return errChangedUnlockHash
	}

	// Update the amount of space remaining to reflect the new volume of total
	// storage.
	h.spaceRemaining += settings.TotalStorage - h.settings.TotalStorage

	h.settings = settings
	return h.save()
}

// Settings returns the settings of a host.
func (h *Host) Settings() modules.HostSettings {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.settings
}
