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
}

// faultyRand is a mocked filesystem which can be configured to fail for certain
// files and folders.
type faultyRand struct {
	productionDependencies
}

var errMockBadRand = errors.New("mocked randomness is intentionally failing")

// Read replaces crypto/rand.Read with a faulty reader - an error is always
// returned.
func (faultyRand) Read([]byte) (int, error) {
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
	h, err := newHost(faultyRand{}, ht.cs, ht.tpool, ht.wallet, ":0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
	err = h.AddStorageFolder(filepath.Join(ht.persistDir, modules.HostDir), minimumStorageFolderSize)
	if err != errMockBadRand {
		t.Fatal(err)
	}
}
