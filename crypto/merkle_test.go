package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

// TestTreeBuilder builds a tree and gets the merkle root.
func TestTreeBuilder(t *testing.T) {
	tree := NewTree()
	tree.PushObject("a")
	tree.PushObject("b")
	_ = tree.Root()

	bytes := [][]byte{{1}, {2}}
	_ = MerkleRoot(bytes)

	// correctness is assumed, as it's tested by the merkletree package. This
	// function is really for code coverage.
}

// TestCalculateLeaves probes the CalulateLeaves function.
func TestCalculateLeaves(t *testing.T) {
	cl0 := CalculateLeaves(0)
	cl63 := CalculateLeaves(63)
	cl64 := CalculateLeaves(64)
	cl65 := CalculateLeaves(65)
	cl127 := CalculateLeaves(127)
	cl128 := CalculateLeaves(128)
	cl129 := CalculateLeaves(129)

	if cl0 != 0 {
		t.Error("miscalculation for cl0")
	}
	if cl63 != 1 {
		t.Error("miscalculation for cl63")
	}
	if cl64 != 1 {
		t.Error("miscalculation for cl64")
	}
	if cl65 != 2 {
		t.Error("miscalculation for cl65")
	}
	if cl127 != 2 {
		t.Error("miscalculation for cl127")
	}
	if cl128 != 2 {
		t.Error("miscalculation for cl128")
	}
	if cl129 != 3 {
		t.Error("miscalculation for cl129")
	}
}

// TestStorageProof builds a storage proof and checks that it verifies
// correctly.
func TestStorageProof(t *testing.T) {
	// generate proof data
	numSegments := uint64(7)
	data := make([]byte, numSegments*SegmentSize)
	rand.Read(data)
	rootHash, err := ReaderMerkleRoot(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	// create and verify proofs for all indices
	for i := uint64(0); i < numSegments; i++ {
		baseSegment, hashSet, err := BuildReaderProof(bytes.NewReader(data), i)
		if err != nil {
			t.Error(err)
			continue
		}
		if !VerifySegment(baseSegment, hashSet, numSegments, i, rootHash) {
			t.Error("Proof", i, "did not pass verification")
		}
	}

	// Try an incorrect proof.
	baseSegment, hashSet, err := BuildReaderProof(bytes.NewReader(data), 3)
	if err != nil {
		t.Fatal(err)
	}
	if VerifySegment(baseSegment, hashSet, numSegments, 4, rootHash) {
		t.Error("Verified a bad proof")
	}
}
