package host

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// faultyFS is a mocked filesystem which can be configured to fail for certain
// files and folders.
type faultyFS struct {
	// brokenSubstrings is a list of substrings that, when appearing in a
	// filepath, will cause the call to fail.
	brokenSubstrings []string

	productionDependencies
}

// TestStorageFolderTolerance tests the tolerance of storage folders in the
// presense of disk failures. Disk failures should be recorded, and the
// failures should be handled gracefully - nonfailing disks should not have
// problems.
func TestStorageFolderTolerance(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := blankHostTester("TestStorageFolderTolerance")
	if err != nil {
		t.Fatal(err)
	}
	// Replace the host with a faulty os.
	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	_, err = newHost(faultyFS{}, ht.cs, ht.tpool, ht.wallet, ":0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}

	// Add a storage folder when the symlinking is failing.
}
