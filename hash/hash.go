package hash

import (
	"crypto/sha256"

	"github.com/NebulousLabs/Sia/encoding"
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

func HashAll(data ...[]byte) Hash {
	bytes := data[0] // will panic if no arguments supplied
	for _, d := range data {
		bytes = append(bytes, d...)
	}
	return HashBytes(bytes)
}

func HashObject(obj interface{}) Hash {
	return HashBytes(encoding.Marshal(obj))
}
