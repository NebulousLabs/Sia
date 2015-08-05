package persist

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
)

// TestIntegrationRandomSuffix checks that the random suffix creator creates
// valid files.
func TestIntegrationRandomSuffix(t *testing.T) {
	tmpDir := build.TempDir("persist", "TestIntegrationRandomSuffix")
	os.MkdirAll(tmpDir, 0700)
	for i := 0; i < 100; i++ {
		suffix := RandomSuffix()
		filename := filepath.Join(tmpDir, "test file - "+suffix+".nil")
		file, err := os.Create(filename)
		if err != nil {
			t.Fatal(err)
		}
		file.Close()
	}
}
