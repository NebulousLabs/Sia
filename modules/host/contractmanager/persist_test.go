package contractmanager

import (
	"os"
	"path/filepath"
	"sync"
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
	for i := 0; i < 500; i++ {
		f1Name := filepath.Join(testDir, "f1")
		f2Name := filepath.Join(testDir, "f2")
		f3Name := filepath.Join(testDir, "f3")
		commonName := filepath.Join(testDir, "common")
		f1Data := []byte{1, 2, 3, 4, 5}
		f2Data := []byte{2, 3, 4, 5, 6}
		f3Data := []byte{4, 5, 1, 2, 3}
		var wg sync.WaitGroup
		wg.Add(3)
		go func() {
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
			wg.Done()
		}()
		go func() {
			file, err := os.Create(f2Name)
			if err != nil {
				t.Error("File write failed:", err)
			}
			_, err = file.Write(f2Data)
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

			err = os.Rename(f2Name, commonName)
			if err != nil {
				t.Error("Rename failed:", err)
			}
			wg.Done()
		}()
		go func() {
			file, err := os.Create(f3Name)
			if err != nil {
				t.Error("File write failed:", err)
			}
			_, err = file.Write(f3Data)
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

			err = os.Rename(f3Name, commonName)
			if err != nil {
				t.Error("Rename failed:", err)
			}
			wg.Done()
		}()
		wg.Wait()
	}
}
