package siacore

import (
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/NebulousLabs/Andromeda/encoding"
	"github.com/NebulousLabs/Andromeda/hash"
)

// maxInt64() returns the maximum possible int64.
func minInt64() int64 {
	maxUint64 := ^uint64(0)
	maxInt64 := int64(maxUint64 >> 1)
	return 0 - maxInt64 - 1
}

// randomInt64() returns a randomly generated int64 from [int64Min, int64Max].
func randomInt64(t *testing.T) int64 {
	// Create a random big.Int covering the full possible range of values for
	// an integer, starting from the value 0.
	bigInt, err := rand.Int(rand.Reader, new(big.Int).SetUint64(^uint64(0)))
	if err != nil {
		t.Fatal(err)
	}

	// Subtract the minimum possible int to adjust the range from [0, uintMax]
	// to [intMin, intMax].
	bigInt.Add(bigInt, big.NewInt(minInt64()))
	return bigInt.Int64()
}

// randomUint64() returns a randomly generated uint64 from [0, uint64Max]
func randomUint64(t *testing.T) uint64 {
	bigInt, err := rand.Int(rand.Reader, new(big.Int).SetUint64(^uint64(0)))
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

// TestBlockEncoding() creates a block entirely full of random values and then
// verifies that they are encoded correctly.
func testBlockMarshalling(t *testing.T) {
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
	encoding.Unmarshal(marshalledBlock, unmarshalledBlock)

	// Check for equality across all fields.
	a := originalBlock
	b := unmarshalledBlock
	if a.ParentBlockID != b.ParentBlockID {
		t.Error(a.ParentBlockID)
		t.Error(b.ParentBlockID)
		t.Fatal("ParentBlockID marshalling problems.")
	}
	if a.Timestamp != b.Timestamp {
		t.Fatal("Timestamp marshalling problems.")
	}
	if a.Nonce != b.Nonce {
		t.Fatal("Nonce marshalling problems.")
	}
	if a.MinerAddress != b.MinerAddress {
		t.Fatal("MinerAddress marshalling problems.")
	}
	if a.MerkleRoot != b.MerkleRoot {
		t.Fatal("MerkleRoot marshalling problems.")
	}
}

// TestTypeMarshalling tries to marshal and unmarshal all types, verifying that
// the marshalling is consistent with the unmarshalling. Right now block is the
// only type implemented, more may be added later.
func TestTypeMarshalling(t *testing.T) {
	// test transactions, create a list of transactions to put in the block.
	testBlockMarshalling(t)
}
