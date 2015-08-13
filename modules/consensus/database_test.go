package consensus

import (
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

// lenSiacoinOutputs returns the size of the siacoin outputs bucket
func (db *setDB) lenSiacoinOutputs() uint64 {
	return db.lenBucket("SiacoinOutputs")
}

// lenFileContracts returns the number of file contracts in the consensus set
func (db *setDB) lenFileContracts() uint64 {
	return db.lenBucket("FileContracts")
}

// lenFCExpirationsHeight returns the number of file contracts which expire at a given height
func (db *setDB) lenFCExpirationsHeight(h types.BlockHeight) uint64 {
	bucketID := append(prefix_fcex, encoding.Marshal(h)...)
	return db.lenBucket(string(bucketID))
}

// lenSiafundOutputs returns the size of the SiafundOutputs bucket
func (db *setDB) lenSiafundOutputs() uint64 {
	return db.lenBucket("SiafundOutputs")
}
