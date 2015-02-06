package crypto

// hash.go supplies a few geneeral hashing functions, using the hashing
// algorithm blake2b. Because changing the hashing algorithm for Sia has much
// stronger implications than changing any of the other algorithms, blake2b is
// the only supported algorithm. Sia is not really flexible enough to support
// multiple.

import (
	"github.com/NebulousLabs/Sia/encoding"

	"github.com/dchest/blake2b"
)

const (
	HashSize = 32
)

type (
	Hash [HashSize]byte
)

// HashAll takes a set of objects as input, encodes them all using the encoding
// package, and then hashes the result.
func HashAll(objs ...interface{}) Hash {
	var b []byte
	for _, obj := range objs {
		b = append(b, encoding.Marshal(obj)...)
	}
	return HashBytes(b)
}

// HashBytes takes a byte slice and returns the result.
func HashBytes(data []byte) Hash {
	return Hash(blake2b.Sum256(data))
}

// HashObject takes an object as input, encodes it using the encoding package,
// and then hashes the result.
func HashObject(obj interface{}) Hash {
	return HashBytes(encoding.Marshal(obj))
}

// JoinHash appends two hashes and then hashes the result.
func JoinHash(left, right Hash) Hash {
	return HashBytes(append(left[:], right[:]...))
}
