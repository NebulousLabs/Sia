package contractmanager

import (
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

// randSector creates a random sector that can be added to the contract
// manager.
func randSector() (root crypto.Hash, data []byte, err error) {
	data, err = crypto.RandBytes(int(modules.SectorSize))
	if err != nil {
		return
	}
	root = crypto.MerkleRoot(data)
	return
}
