package hash

import (
	"crypto/sha256"

	"github.com/NebulousLabs/Sia/encoding"
)

const (
	HashSize = 32
)

type (
	Hash [HashSize]byte
)

func HashBytes(data []byte) Hash {
	return sha256.Sum256(data)
}

func HashObject(obj interface{}) Hash {
	return HashBytes(encoding.Marshal(obj))
}
