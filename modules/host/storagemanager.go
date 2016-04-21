package host

// TODO: Need to explain the whole 'expiryHeight' thing.

import (
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// A StorageManager is responsible for managing storage folders and sectors for
// the host.
type StorageManager interface {
	// AddSector will add a sector to the storage manager. If the sector
	// already exists, a virtual sector will be added, meaning that the
	// 'sectorData' will be ignored and no new disk space will be consumed.
	AddSector(sectorRoot crypto.Hash, expiryHeight types.BlockHeight, sectorData []byte) error

	// ReadSector will read a sector from the storage manager, returning the
	// bytes that match the input sector root.
	ReadSector(sectorRoot crypto.Hash) ([]byte, error)

	// RemoveSector will remove a sector from the storage manager.
	RemoveSector(sectorRoot crypto.Hash, expiryHeight types.BlockHeight) error

	modules.StorageFolderManager
}
