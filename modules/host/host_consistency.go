package host

import (
	"errors"
)

// consistency.go contians a bunch of consistency checks for the host. Because
// a lot of the consistency is checking that different parts of the host match
// up, it was decided that all consistency checking should go into a single
// file. As an example, the state of the storage obligations sets the standard
// for what it means for the storage folders to be consistent. And the storage
// folders set the standard for what it means for the sectors to be consistent.
//
// Consistency checks should be run infrequently, and should take only a minute
// or two to complete, even for a host that has millions of sectors. This means
// that certain consistency checks are off-limits.

// TODO: Consistency checks should be accompanied by repair tools. Perhaps that
// means instead of calling build.Critical... it just returns some sort of
// diagnostic?

var (
	// errStorageFolderMaxSizeExceeded is returned when a storage folder is
	// found to have exceeded the maximum set by the build constants.
	errStorageFolderMaxSizeExceeded = errors.New("storage folder has exceeded the maximum allowed size")

	// errStorageFolderMinSizeViolated is returned when a storage folder has a
	// size which is smaller than the minimum set by the build constants.
	errStorageFolderMinSizeViolated = errors.New("storage folder has less storage than the minimum allowed")

	// errStorageFolderSizeRemainingDivergence is returned when a storage
	// folder has an amount of storage remaining that indicates an impossible
	// state, such as having more storage remaining than total storage
	// available.
	errStorageFolderSizeRemainingDivergence = errors.New("storage folder has an impossible size remaining value")

	// errStorageFolderInvalidUIDLen is returned when a storage folder has a
	// UID which has the wrong length.
	errStorageFolderInvalidUIDLen = errors.New("storage folder has a UID with an invalid length, often indicating the wrong build of 'siad' is being used")

	// errStorageFolderDuplicateUID is returned when a storage folder has a UID
	// which is known to already be owned by a different storage folder.
	errStorageFolderDuplicateUID = errors.New("storage folder has a UID which is already owned by another storage folder")
)

// storageFolderSizeConsistency checks that all of the storage folders have
// sane sizes.
func (h *Host) storageFolderSizeConsistency() error {
	knownUIDs := make(map[string]int)
	for i, sf := range h.storageFolders {
		// The size of a storage folder should be between the minimum and the
		// maximum allowed size.
		if sf.Size > maximumStorageFolderSize {
			h.log.Critical("storage folder", i, "exceeds the maximum allowed storage folder size")
			return errStorageFolderMaxSizeExceeded
		}
		if sf.Size < minimumStorageFolderSize {
			h.log.Critical("storage folder", i, "has less than the minimum allowed storage folder size")
			return errStorageFolderMinSizeViolated
		}

		// The amount of storage remaining should not be greater than the
		// folder size.
		if sf.SizeRemaining > sf.Size {
			h.log.Critical("storage folder", i, "has more storage remaining than it has storage total")
			return errStorageFolderSizeRemainingDivergence
		}

		// The UID has a fixed size.
		if len(sf.UID) != storageFolderUIDSize {
			h.log.Critical("storage folder", i, "has an ID which is not valid")
			return errStorageFolderInvalidUIDLen
		}

		// Check that the storage folder UID is not conflicting with any other
		// known storage folder UID.
		conflict, exists := knownUIDs[sf.uidString()]
		if exists {
			h.log.Critical("storage folder", i, "has a duplicate UID, conflicting with storage folder", conflict)
			return errStorageFolderDuplicateUID
		}
		// Add this storage folder's UID to the set of known UIDs.
		knownUIDs[sf.uidString()] = i
	}
	return nil
}
