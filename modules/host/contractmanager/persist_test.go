package contractmanager

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
)

// TestOSRename tries several different aggressive file renaming strategies.
func TestOSRename(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create the directory for this test.
	testDir := build.TempDir("contractmanager", t.Name())
	err := os.MkdirAll(testDir, 0700)
	if err != nil {
		t.Fatal(err)
	}

	// Create two different files, and then rename them in parallel to the same
	// file.
	for i := 0; i < 25000; i++ {
		f1Name := filepath.Join(testDir, "f1")
		commonName := filepath.Join(testDir, "common")
		f1Data := []byte{1, 2, 3, 4, 5}
		file, err := os.Create(f1Name)
		if err != nil {
			t.Error("File write failed:", err)
		}
		_, err = file.Write(f1Data)
		if err != nil {
			t.Error("File write failed:", err)
		}
		err = file.Sync()
		if err != nil {
			t.Error("File write failed:", err)
		}
		err = file.Close()
		if err != nil {
			t.Error("File write failed:", err)
		}

		err = os.Rename(f1Name, commonName)
		if err != nil {
			t.Error("Rename failed:", err)
		}
	}
}
