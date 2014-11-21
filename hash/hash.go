package hash

import (
	"crypto/sha256"
)

const (
	HashSize    = 32
	SegmentSize = 64 // Size of smallest piece of a file which gets hashed when building the Merkle tree.
)

type (
	Hash [HashSize]byte
)

func HashBytes(data []byte) Hash {
	return sha256.Sum256(data)
}
