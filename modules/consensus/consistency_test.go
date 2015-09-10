package consensus

import (
	"github.com/boltdb/bolt"

	"github.com/NebulousLabs/Sia/crypto"
)

// dbConsensusChecksum is a convenience function to call consensusChecksum
// without a bolt.Tx.
func (cs *ConsensusSet) dbConsensusChecksum() (checksum crypto.Hash) {
	_ = cs.db.Update(func(tx *bolt.Tx) error {
		checksum = consensusChecksum(tx)
		return nil
	})
	return checksum
}
