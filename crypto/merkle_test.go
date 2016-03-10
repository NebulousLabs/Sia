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

	// Correctness is assumed, as it's tested by the merkletree package. This
	// function is really for code coverage.
}

// TestCalculateLeaves probes the CalulateLeaves function.
func TestCalculateLeaves(t *testing.T) {
	tests := []struct {
		size, expSegs uint64
	}{
		{0, 1},
		{63, 1},
		{64, 1},
		{65, 2},
		{127, 2},
		{128, 2},
		{129, 3},
	}

	for i, test := range tests {
		if segs := CalculateLeaves(test.size); segs != test.expSegs {
			t.Errorf("miscalculation for test %v: expected %v, got %v", i, test.expSegs, segs)
		}
	}
}

// TestStorageProof builds a storage proof and checks that it verifies
// correctly.
func TestStorageProof(t *testing.T) {
	// Generate proof data.
	numSegments := uint64(7)
	data := make([]byte, numSegments*SegmentSize)
	rand.Read(data)
	rootHash, err := ReaderMerkleRoot(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	// Create and verify proofs for all indices.
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

// TestNonMultipleNumberOfSegmentsStorageProof builds a storage proof that has
// a last leaf of size less than SegmentSize.
func TestNonMultipleLeafSizeStorageProof(t *testing.T) {
	// Generate proof data.
	data := make([]byte, (2*SegmentSize)+10)
	rand.Read(data)
	rootHash, err := ReaderMerkleRoot(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	// Create and verify a proof for the last index.
	baseSegment, hashSet, err := BuildReaderProof(bytes.NewReader(data), 2)
	if err != nil {
		t.Error(err)
	}
	if !VerifySegment(baseSegment, hashSet, 3, 2, rootHash) {
		t.Error("padded segment proof failed")
	}
}

// TestReadSegments tests MerkleTree.ReadSegments by building a MerkleTree and
// comparing its root with the root returned from ReaderMerkleRoot.
func TestReadSegments(t *testing.T) {
	// Generate leaf data.
	numSegments := 7
	data := make([]byte, numSegments*SegmentSize)
	rand.Read(data)

	// Generate merkle tree root using MerkleTree.ReadSegments and
	// MerkleTree.Root.
	m := NewTree()
	err := m.ReadSegments(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	rootHash := m.Root()

	// Generate merkle tree root using ReaderMerkleRoot.
	expectedHash, err := ReaderMerkleRoot(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	// Compare roots.
	if rootHash != expectedHash {
		t.Errorf("root hashes not consistent: expected %s, got %s\n", expectedHash, rootHash)
	}
}

// TestNonMultipleNumberOfSegmentsReadSegments tests MerkleTree.ReadSements for
// data that doesn't fit evently into a whole number of leaves. It does so by
// building a MerkleTree with MerkleTree.ReadSegments and comparing its root
// with the root returned from ReaderMerkleRoot.
func TestNonMultipleNumberOfSegmentsReadSegments(t *testing.T) {
	// Generate leaf data that doesn't fit evenly into leaves.
	numSegments := 2.5
	data := make([]byte, int(numSegments*SegmentSize))
	rand.Read(data)

	// Generate merkle tree root using MerkleTree.ReadSegments and
	// MerkleTree.Root.
	m := NewTree()
	err := m.ReadSegments(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	rootHash := m.Root()

	// Generate merkle tree root using ReaderMerkleRoot.
	expectedHash, err := ReaderMerkleRoot(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	// Compare roots.
	if rootHash != expectedHash {
		t.Errorf("root hashes not consistent: expected %s, got %s\n", expectedHash, rootHash)
	}
}
