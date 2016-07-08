package host

import (
	"errors"

	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

var (
	// errObligationLocked is returned if the file contract being requested is
	// currently locked. The lock can be in place if there is a storage proof
	// being submitted, if there is another renter altering the contract, or if
	// there have been network connections with have not resolved yet.
	errObligationLocked = errors.New("the requested file contract is currently locked")
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
		return errObligationLocked
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
