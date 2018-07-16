package host

import (
	"errors"

	"gitlab.com/NebulousLabs/Sia/sync"
	"gitlab.com/NebulousLabs/Sia/types"
)

var (
	// errObligationLocked is returned if the file contract being requested is
	// currently locked. The lock can be in place if there is a storage proof
	// being submitted, if there is another renter altering the contract, or if
	// there have been network connections with have not resolved yet.
	errObligationLocked = errors.New("the requested file contract is currently locked")
)

// managedLockStorageObligation puts a storage obligation under lock in the
// host.
func (h *Host) managedLockStorageObligation(soid types.FileContractID) {
	// Check if a lock has been created for this storage obligation. If not,
	// create one. The map must be accessed under lock, but the request for the
	// storage lock must not be made under lock.
	h.mu.Lock()
	tl, exists := h.lockedStorageObligations[soid]
	if !exists {
		tl = new(sync.TryMutex)
		h.lockedStorageObligations[soid] = tl
	}
	h.mu.Unlock()

	tl.Lock()
}

// managedTryLockStorageObligation attempts to put a storage obligation under
// lock, returning an error if the lock cannot be obtained.
func (h *Host) managedTryLockStorageObligation(soid types.FileContractID) error {
	// Check if a lock has been created for this storage obligation. If not,
	// create one. The map must be accessed under lock, but the request for the
	// storage lock must not be made under lock.
	h.mu.Lock()
	tl, exists := h.lockedStorageObligations[soid]
	if !exists {
		tl = new(sync.TryMutex)
		h.lockedStorageObligations[soid] = tl
	}
	h.mu.Unlock()

	if tl.TryLockTimed(obligationLockTimeout) {
		return nil
	}
	return errObligationLocked
}

// managedUnlockStorageObligation takes a storage obligation out from under lock in
// the host.
func (h *Host) managedUnlockStorageObligation(soid types.FileContractID) {
	// Check if a lock has been created for this storage obligation. The map
	// must be accessed under lock, but the request for the unlock must not
	// be made under lock.
	h.mu.Lock()
	tl, exists := h.lockedStorageObligations[soid]
	if !exists {
		h.log.Critical(errObligationUnlocked)
		return
	}
	h.mu.Unlock()

	tl.Unlock()
}
