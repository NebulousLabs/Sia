package modules

// TODO: Need to explain the whole 'expiryHeight' thing.

// TODO: Need to update the documentation to reflect the structural change
// between the host and the storage manager.

import (
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// StorageManagerDir is standard name used for the directory that contains
	// all of the storage manager files.
	StorageManagerDir = "storagemanager"
)

type (
	// StorageFolderMetadata contians metadata about a storage folder that is
	// tracked by the storage folder manager.
	StorageFolderMetadata struct {
		Capacity          uint64 `json:"capacity"`
		CapacityRemaining uint64 `json:"capacityremaining"`
		Path              string `json:"path"`

		// Below are statistics about the filesystem. FailedReads and
		// FailedWrites are only incremented if the filesystem is returning
		// errors when operations are being performed. A large number of
		// FailedWrites can indicate that more space has been allocated on a
		// drive than is physically available. A high number of failures can
		// also indicaate disk trouble.
		FailedReads      uint64 `json:"failedreads"`
		FailedWrites     uint64 `json:"failedwrites"`
		SuccessfulReads  uint64 `json:"successfulreads"`
		SuccessfulWrites uint64 `json:"successfulwrites"`
	}

	// A StorageManager is responsible for managing storage folders and sectors for
	// the host.
	StorageManager interface {
		// AddSector will add a sector to the storage manager. If the sector
		// already exists, a virtual sector will be added, meaning that the
		// 'sectorData' will be ignored and no new disk space will be consumed.
		AddSector(sectorRoot crypto.Hash, expiryHeight types.BlockHeight, sectorData []byte) error

		// AddStorageFolder adds a storage folder to the host. The host may not
		// check that there is enough space available on-disk to support as
		// much storage as requested, though the host should gracefully handle
		// running out of storage unexpectedly.
		AddStorageFolder(path string, size uint64) error

		// The storage manager needs to be able to shut down.
		Close() error

		// DeleteSector deletes a sector, meaning that the host will be unable
		// to upload that sector and be unable to provide a storage proof on
		// that sector. This function is not intended to be used, but is
		// available in case a host is compelled by their government to delete
		// a piece of illegal data.
		DeleteSector(sectorRoot crypto.Hash) error

		// ReadSector will read a sector from the storage manager, returning the
		// bytes that match the input sector root.
		ReadSector(sectorRoot crypto.Hash) ([]byte, error)

		// RemoveSector will remove a sector from the storage manager.
		RemoveSector(sectorRoot crypto.Hash, expiryHeight types.BlockHeight) error

		// RemoveStorageFolder will remove a storage folder from the host. All
		// storage on the folder will be moved to other storage folders,
		// meaning that no data will be lost. If the host is unable to save
		// data, an error will be returned and the operation will be stopped.
		RemoveStorageFolder(index int, force bool) error

		// ResetStorageFolderHealth will reset the health statistics on a
		// storage folder.
		ResetStorageFolderHealth(index int) error

		// ResizeStorageFolder will grow or shrink a storage folder in the
		// host. The host may not check that there is enough space on-disk to
		// support growing the storage folder, but should gracefully handle
		// running out of space unexpectedly. When shrinking a storage folder,
		// any data in the folder that needs to be moved will be placed into
		// other storage folders, meaning that no data will be lost. If the
		// host is unable to migrate the data, an error will be returned and
		// the operation will be stopped.
		ResizeStorageFolder(index int, newSize uint64) error

		// StorageFolders will return a list of storage folders tracked by the
		// host.
		StorageFolders() ([]StorageFolderMetadata, error)
	}
)
