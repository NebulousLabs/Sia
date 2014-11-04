package sia

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestMerkleRoot(t *testing.T) {
	// compare MerkleRoot fn to manual hashing
	leaves := []Hash{Hash{0}, Hash{1}, Hash{2}, Hash{3}, Hash{4}, Hash{5}, Hash{6}, Hash{7}}

	root8 := joinHash(
		joinHash(
			joinHash(leaves[0], leaves[1]),
			joinHash(leaves[2], leaves[3]),
		),
		joinHash(
			joinHash(leaves[4], leaves[5]),
			joinHash(leaves[6], leaves[7]),
		),
	)
	root7 := joinHash(
		joinHash(
			joinHash(leaves[0], leaves[1]),
			joinHash(leaves[2], leaves[3]),
		),
		joinHash(
			joinHash(leaves[4], leaves[5]),
			leaves[6],
		),
	)
	root6 := joinHash(
		joinHash(
			joinHash(leaves[0], leaves[1]),
			joinHash(leaves[2], leaves[3]),
		),
		joinHash(leaves[4], leaves[5]),
	)
	root5 := joinHash(
		joinHash(
			joinHash(leaves[0], leaves[1]),
			joinHash(leaves[2], leaves[3]),
		),
		leaves[4],
	)

	if root8 != MerkleRoot(leaves) {
		t.Fatal("MerkleRoot hash does not match manual hash (8 leaves)")
	}
	if root7 != MerkleRoot(leaves[:7]) {
		t.Fatal("MerkleRoot hash does not match manual hash (7 leaves)")
	}
	if root6 != MerkleRoot(leaves[:6]) {
		t.Fatal("MerkleRoot hash does not match manual hash (6 leaves)")
	}
	if root5 != MerkleRoot(leaves[:5]) {
		t.Fatal("MerkleRoot hash does not match manual hash (5 leaves)")
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
