package hash

import (
	"github.com/NebulousLabs/Sia/encoding"

	"github.com/codahale/blake2"
)

const (
	HashSize = 32
)

type (
	Hash [HashSize]byte
)

func HashBytes(data []byte) (hash Hash) {
	hasher := blake2.New(&blake2.Config{Size: HashSize})
	hasher.Write(data)
	sum := hasher.Sum(nil)
	copy(hash[:], sum)
	return
}

func HashObject(obj interface{}) Hash {
	return HashBytes(encoding.Marshal(obj))
}
