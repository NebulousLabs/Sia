package crypto

// hash.go supplies a few geneeral hashing functions, using the hashing
// algorithm blake2b. Because changing the hashing algorithm for Sia has much
// stronger implications than changing any of the other algorithms, blake2b is
// the only supported algorithm. Sia is not really flexible enough to support
// multiple.

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"

	"github.com/NebulousLabs/Sia/encoding"

	"github.com/dchest/blake2b"
)

const (
	HashSize = 32
)

type (
	Hash [HashSize]byte

	// HashSlice is used for sorting
	HashSlice []Hash
)

var (
	ErrHashWrongLen = errors.New("encoded value has the wrong length to be a hash")
)

// NewHash returns a blake2b 256bit hasher.
func NewHash() hash.Hash {
	return blake2b.New256()
}

// HashAll takes a set of objects as input, encodes them all using the encoding
// package, and then hashes the result.
func HashAll(objs ...interface{}) Hash {
	// Ideally we would just write HashBytes(encoding.MarshalAll(objs)).
	// Unfortunately, you can't pass 'objs' to MarshalAll without losing its
	// type information; MarshalAll would just see interface{}s.
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

// These functions implement sort.Interface, allowing hashes to be sorted.
func (hs HashSlice) Len() int           { return len(hs) }
func (hs HashSlice) Less(i, j int) bool { return bytes.Compare(hs[i][:], hs[j][:]) < 0 }
func (hs HashSlice) Swap(i, j int)      { hs[i], hs[j] = hs[j], hs[i] }

// MarshalJSON marshales a hash as a hex string.
func (h Hash) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.String())
}

// String prints the hash in hex.
func (h Hash) String() string {
	return hex.EncodeToString(h[:])
}

// UnmarshalJSON decodes the json hex string of the hash.
func (h *Hash) UnmarshalJSON(b []byte) error {
	// *2 because there are 2 hex characters per byte.
	// +2 because the encoded JSON string has a `"` added at the beginning and end.
	if len(b) != HashSize*2+2 {
		return ErrHashWrongLen
	}

	// b[1 : len(b)-1] cuts off the leading and trailing `"` in the JSON string.
	hBytes, err := hex.DecodeString(string(b[1 : len(b)-1]))
	if err != nil {
		return errors.New("could not unmarshal crypto.Hash: " + err.Error())
	}
	copy(h[:], hBytes)
	return nil
}
