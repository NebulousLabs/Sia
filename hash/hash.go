package hash

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

func HashAll(objs ...interface{}) Hash {
	var b []byte
	for _, obj := range objs {
		b = append(b, encoding.Marshal(obj)...)
	}
	return HashBytes(b)
}

func HashBytes(data []byte) (hash Hash) {
	return Hash(blake2b.Sum256(data))
}

func HashObject(obj interface{}) Hash {
	return HashBytes(encoding.Marshal(obj))
}
