package sia

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestMerkleRoot(t *testing.T) {
	// compare MerkleRoot fn to manual hashing of a 7-leaf tree
	leaves := []Hash{Hash{0}, Hash{1}, Hash{2}, Hash{3}, Hash{4}, Hash{5}, Hash{6}}

	// calculate root manually
	manualRoot := joinHash(
		joinHash(
			joinHash(leaves[0], leaves[1]),
			joinHash(leaves[2], leaves[3]),
		),
		joinHash(
			joinHash(leaves[4], leaves[5]),
			leaves[6],
		),
	)

	if manualRoot != MerkleRoot(leaves) {
		t.Fatal("MerkleRoot hash does not match manual hash")
	}
}

func TestStorageProof(t *testing.T) {
	// generate proof data
	var numSegments uint16 = 7
	data := make([]byte, numSegments*SegmentSize)
	rand.Read(data)
	rootHash, err := MerkleFile(bytes.NewReader(data), numSegments)
	if err != nil {
		t.Fatal(err)
	}

	// create and verify proofs for all indices
	for i := uint16(0); i < numSegments; i++ {
		sp, err := buildProof(bytes.NewReader(data), numSegments, i)
		if err != nil {
			t.Error(err)
			continue
		}
		if !verifyProof(sp, numSegments, i, rootHash) {
			t.Error("Proof", i, "did not pass verification")
		}
	}
}
