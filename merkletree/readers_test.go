package merkletree

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

// TestReaderMerkleRoot calls ReaderMerkleRoot on a manually crafted dataset
// and checks the output.
func TestReaderMerkleRoot(t *testing.T) {
	mt := CreateMerkleTester(t)
	bytes8 := []byte{0, 1, 2, 3, 4, 5, 6, 7}
	reader := bytes.NewReader(bytes8)
	root, err := ReaderMerkleRoot(reader, sha256.New(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(root, mt.roots[8]) != 0 {
		t.Error("ReaderMerkleRoot returned the wrong root")
	}
}

// TestReaderMerkleRootEOF passes ReaderMerkleRoot a reader that has too few
// bytes to perfectly fill the segment size. The reader should pad the final
// segment with zeros.
func TestReaderMerkleRootEOF(t *testing.T) {
	bytes1 := []byte{1}
	reader := bytes.NewReader(bytes1)
	root, err := ReaderMerkleRoot(reader, sha256.New(), 2)
	if err != nil {
		t.Fatal(err)
	}

	expectedRoot := sum(sha256.New(), []byte{1, 0})
	if bytes.Compare(root, expectedRoot) != 0 {
		t.Error("ReaderMerkleRoot returned the wrong root")
	}
}

// TestBuildReaderProof calls BuildReaderProof on a manually crafted dataset
// and checks the output.
func TestBuilReaderProof(t *testing.T) {
	mt := CreateMerkleTester(t)
	bytes7 := []byte{0, 1, 2, 3, 4, 5, 6}
	reader := bytes.NewReader(bytes7)
	root, proveSet, numLeaves, err := BuildReaderProof(reader, sha256.New(), 1, 5)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(root, mt.roots[7]) != 0 {
		t.Error("BuildReaderProof returned the wrong root")
	}
	if len(proveSet) != len(mt.proveSets[7][5]) {
		t.Fatal("BuildReaderProof returned a proof with the wrong length")
	}
	for i := range proveSet {
		if bytes.Compare(proveSet[i], mt.proveSets[7][5][i]) != 0 {
			t.Error("BuildReaderProof returned an incorrect proof")
		}
	}
	if numLeaves != 7 {
		t.Error("BuildReaderProof returned the wrong number of leaves")
	}
}

// TestBuilderProofEOF passes BuildReaderProof a reader that has too few bytes
// to perfectly fill the segment size. The reader should pad the final segment
// with zeros.
func TestBuilderProofEOF(t *testing.T) {
	bytes1 := []byte{1}
	reader := bytes.NewReader(bytes1)
	root, proveSet, numLeaves, err := BuildReaderProof(reader, sha256.New(), 2, 0)
	if err != nil {
		t.Fatal(err)
	}

	expectedRoot := sum(sha256.New(), []byte{1, 0})
	if bytes.Compare(root, expectedRoot) != 0 {
		t.Error("ReaderMerkleRoot returned the wrong root")
	}
	if len(proveSet) != 1 {
		t.Fatal("proveSet is the incorrect lenght")
	}
	if bytes.Compare(proveSet[0], []byte{1, 0}) != 0 {
		t.Error("prove set is incorrect")
	}
	if numLeaves != 1 {
		t.Error("wrong number of leaves returned")
	}
}
