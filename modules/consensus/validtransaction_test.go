package consensus

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestStorageProofSegment probes the storageProofSegment method of the
// consensus set.
func TestStorageProofSegment(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestStorageProofSegment")
	if err != nil {
		t.Fatal(err)
	}

	// Add a file contract to the consensus set that can be used to probe the
	// storage segment.
	var outputs []byte
	for i := 0; i < 4*256*256; i++ {
		var fcid types.FileContractID
		rand.Read(fcid[:])
		fc := types.FileContract{
			WindowStart: 2,
			FileSize:    256 * 64,
		}
		cst.cs.fileContracts[fcid] = fc
		index, err := cst.cs.storageProofSegment(fcid)
		if err != nil {
			t.Error(err)
		}
		outputs = append(outputs, byte(index))
	}

	// Perform entropy testing on 'outputs' to verify randomness.
	var b bytes.Buffer
	zip := gzip.NewWriter(&b)
	_, err = zip.Write(outputs)
	if err != nil {
		t.Fatal(err)
	}
	zip.Close()
	if b.Len() < len(outputs) {
		t.Error("supposedly high entropy random segments have been compressed!")
	}
}
