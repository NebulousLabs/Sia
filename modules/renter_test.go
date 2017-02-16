package modules

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/persist"
)

// TestMerkleRootSetCompatibility checks that the persist encoding for the
// MerkleRootSet type is compatible with the previous encoding for the data,
// which was a slice of type crypto.Hash.
func TestMerkleRootSetCompatibility(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create some fake headers for the files.
	meta := persist.Metadata{
		Header:  "Test Header",
		Version: "0.1.2",
	}

	// Try multiple sizes of array.
	for i := 0; i < 10; i++ {
		// Create a []crypto.Hash of length i.
		type chStruct struct {
			Hashes []crypto.Hash
		}
		var chs chStruct
		for j := 0; j < i; j++ {
			var ch crypto.Hash
			_, err := rand.Read(ch[:])
			if err != nil {
				t.Fatal(err)
			}
			chs.Hashes = append(chs.Hashes, ch)
		}

		// Save and load, check that they are the same.
		dir := build.TempDir("modules", "testMerkleRootSetCompatibility")
		err := os.MkdirAll(dir, 0700)
		if err != nil {
			t.Fatal(err)
		}
		filename := filepath.Join(dir, "file")
		err = persist.SaveFile(meta, chs, filename)
		if err != nil {
			t.Fatal(err)
		}

		// Load and verify equivalence.
		var loadCHS chStruct
		err = persist.LoadFile(meta, &loadCHS, filename)
		if err != nil {
			t.Fatal(err)
		}
		if len(chs.Hashes) != len(loadCHS.Hashes) {
			t.Fatal("arrays should be the same size")
		}
		for j := range chs.Hashes {
			if chs.Hashes[j] != loadCHS.Hashes[j] {
				t.Error("loading failed", i, j)
			}
		}

		// Load into MerkleRootSet and verify equivalence.
		type mrStruct struct {
			Hashes MerkleRootSet
		}
		var loadMRS mrStruct
		err = persist.LoadFile(meta, &loadMRS, filename)
		if err != nil {
			t.Fatal(err)
		}
		if len(chs.Hashes) != len(loadMRS.Hashes) {
			t.Fatal("arrays should be the same size")
		}
		for j := range chs.Hashes {
			if chs.Hashes[j] != loadMRS.Hashes[j] {
				t.Error("loading failed", i, j)
			}
		}

		// Save as a MerkleRootSet and verify it can be loaded again.
		var mrs mrStruct
		mrs.Hashes = MerkleRootSet(chs.Hashes)
		err = persist.SaveFile(meta, mrs, filename)
		if err != nil {
			t.Fatal(err)
		}
		err = persist.LoadFile(meta, &loadMRS, filename)
		if err != nil {
			t.Fatal(err)
		}
		if len(mrs.Hashes) != len(loadMRS.Hashes) {
			t.Fatal("arrays should be the same size")
		}
		for j := range mrs.Hashes {
			if mrs.Hashes[j] != loadMRS.Hashes[j] {
				t.Error("loading failed", i, j)
			}
		}
	}
}
