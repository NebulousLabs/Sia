package consensus

import (
	"crypto/rand"
	"math"
	"math/big"
	"testing"

	"github.com/NebulousLabs/Andromeda/encoding"
	"github.com/NebulousLabs/Andromeda/hash"
)

// randomInt64() returns a randomly generated int64 from [int64Min, int64Max].
func randomInt64(t *testing.T) int64 {
	// Create a random big.Int covering the full possible range of values for
	// an integer, starting from the value 0.
	bigInt, err := rand.Int(rand.Reader, new(big.Int).SetUint64(math.MaxInt64))
	if err != nil {
		t.Fatal(err)
	}
	return bigInt.Int64()
}

// randomUint64() returns a randomly generated uint64 from [0, uint64Max]
func randomUint64(t *testing.T) uint64 {
	bigInt, err := rand.Int(rand.Reader, new(big.Int).SetUint64(math.MaxUint64))
	if err != nil {
		t.Fatal(err)
	}
	return bigInt.Uint64()
}

// randomHash() returns a hash.Hash filled with entirely random values.
func randomHash(t *testing.T) (h hash.Hash) {
	n, err := rand.Read(h[:])
	if err != nil {
		t.Fatal(n, "::", err)
	}
	return
}

// TestTypeMarshalling tries to marshal and unmarshal all types, verifying that
// the marshalling is consistent with the unmarshalling. Right now block is the
// only type implemented, more may be added later.
func TestTypeMarshalling(t *testing.T) {
	// Create a block full of random values.
	originalBlock := Block{
		ParentBlockID: BlockID(randomHash(t)),
		Timestamp:     Timestamp(randomInt64(t)),
		Nonce:         randomUint64(t),
		MinerAddress:  CoinAddress(randomHash(t)),
		MerkleRoot:    randomHash(t),
		// Transactions to be added from... input?
	}

	marshalledBlock := encoding.Marshal(originalBlock)
	var unmarshalledBlock Block
	encoding.Unmarshal(marshalledBlock, &unmarshalledBlock)

	// Check for equality across all fields.
	a := originalBlock
	b := unmarshalledBlock
	if a.ParentBlockID != b.ParentBlockID {
		t.Error("ParentBlockID marshalling problems.")
	}
	if a.Timestamp != b.Timestamp {
		t.Error("Timestamp marshalling problems.")
	}
	if a.Nonce != b.Nonce {
		t.Error("Nonce marshalling problems.")
	}
	if a.MinerAddress != b.MinerAddress {
		t.Error("MinerAddress marshalling problems.")
	}
	if a.MerkleRoot != b.MerkleRoot {
		t.Error("MerkleRoot marshalling problems.")
	}
}
