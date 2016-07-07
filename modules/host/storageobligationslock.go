package host

import (
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

// lockStorageObligation puts a storage obligation under lock in the host.
func (h *Host) lockStorageObligation(soid types.FileContractID) {
	tl, exists := h.lockedStorageObligations[soid]
	if exists {
		tl.Lock()
		return
	}
	tl = new(sync.TryMutex)
	tl.Lock()
	h.lockedStorageObligations[soid] = tl
}

// tryLockStorageObligation attempts to put a storage obligation under lock,
// returning an error if the lock cannot be obtained.
func (h *Host) tryLockStorageObligation(soid types.FileContractID) error {
	tl, exists := h.lockedStorageObligations[soid]
	if exists {
		return tl.TryLock()
	}
	tl = new(sync.TryMutex)
	tl.Lock()
	h.lockedStorageObligations[soid] = tl
	return nil
}

// unlockStorageObligation takes a storage obligation out from under lock in
// the host.
func (h *Host) unlockStorageObligation(soid types.FileContractID) {
	tl, exists := h.lockedStorageObligations[soid]
	if !exists {
		h.log.Critical(errObligationUnlocked)
		return
	}
	tl.Unlock()
}
