package host

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// TestClosedHostOperations tries a bunch of operations on the host after it
// has been closed.
func TestClosedHostOperations(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := blankHostTester("TestClosedHostOperations")
	if err != nil {
		t.Fatal(err)
	}
	// Close the host.
	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}

	err = ht.host.AddStorageFolder(filepath.Join(ht.persistDir, modules.HostDir), minimumStorageFolderSize)
	if err != errHostClosed {
		t.Fatal("expected errHostClosed:", err)
	}
	err = ht.host.RemoveStorageFolder(1, true)
	if err != errHostClosed {
		t.Fatal("expected errHostClosed:", err)
	}
	err = ht.host.ResizeStorageFolder(0, minimumStorageFolderSize)
	if err != errHostClosed {
		t.Fatal("expected errHostClosed:", err)
	}
	// Number of storage folders should still be zero.
	if len(ht.host.storageFolders) != 0 {
		t.Error("storage folder should not have been added to the host as the host is closed.")
	}
}

// faultyRand is a mocked filesystem which can be configured to fail for certain
// files and folders.
type faultyRand struct {
	productionDependencies
}

// errMockBadRand is returned when a mocked dependency is intentionally
// returning an error instead of randomly generating data.
var errMockBadRand = errors.New("mocked randomness is intentionally failing")

// randRead replaces the production dependency crypto/rand.Read with a faulty
// reader - an error is always returned.
func (faultyRand) randRead([]byte) (int, error) {
	return 0, errMockBadRand
}

// TestAddFolderNoRand tries adding a folder to the host when the cryptographic
// randomness generator is not working.
func TestAddFolderNoRand(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := blankHostTester("TestAddFolderNoRand")
	if err != nil {
		t.Fatal(err)
	}
	// Replace the host with one that cannot do randomness operations.
	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	ht.host, err = newHost(faultyRand{}, ht.cs, ht.tpool, ht.wallet, ":0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.AddStorageFolder(filepath.Join(ht.persistDir, modules.HostDir), minimumStorageFolderSize)
	if err != errMockBadRand {
		t.Fatal(err)
	}
	// Number of storage folders should be zero.
	if len(ht.host.storageFolders) != 0 {
		t.Error("storage folder was added to the host despite a dependency failure")
	}
}
