package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

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
}
