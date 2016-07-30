package contractmanager

/*
import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// TestClosedStorageManagerOperations tries a bunch of operations on the storage manager
// after it has been closed.
func TestClosedStorageManagerOperations(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	smt, err := newStorageManagerTester("TestClosedStorageManagerOperations")
	if err != nil {
		t.Fatal(err)
	}
	// Close the storage manager.
	err = smt.sm.Close()
	if err != nil {
		t.Fatal(err)
	}
	defer smt.Close()

	err = smt.sm.AddStorageFolder(filepath.Join(smt.persistDir, modules.StorageManagerDir), minimumStorageFolderSize)
	if err != errStorageManagerClosed {
		t.Fatal("expected errStorageManagerClosed:", err)
	}
	err = smt.sm.RemoveStorageFolder(1, true)
	if err != errStorageManagerClosed {
		t.Fatal("expected errStorageManagerClosed:", err)
	}
	err = smt.sm.ResizeStorageFolder(0, minimumStorageFolderSize)
	if err != errStorageManagerClosed {
		t.Fatal("expected errStorageManagerClosed:", err)
	}
	// Number of storage folders should still be zero.
	if len(smt.sm.storageFolders) != 0 {
		t.Error("storage folder should not have been added to the storage manager as the storage manager is closed.")
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

// TestAddFolderNoRand tries adding a folder to the storage manager when the
// cryptographic randomness generator is not working.
func TestAddFolderNoRand(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	smt, err := newStorageManagerTester("TestAddFolderNoRand")
	if err != nil {
		t.Fatal(err)
	}
	defer smt.Close()

	// Replace the storage manager with one that cannot do randomness
	// operations.
	err = smt.sm.Close()
	if err != nil {
		t.Fatal(err)
	}
	smt.sm, err = newStorageManager(faultyRand{}, filepath.Join(smt.persistDir, modules.StorageManagerDir))
	if err != nil {
		t.Fatal(err)
	}
	err = smt.sm.AddStorageFolder(filepath.Join(smt.persistDir, modules.StorageManagerDir), minimumStorageFolderSize)
	if err != errMockBadRand {
		t.Fatal(err)
	}
	// Number of storage folders should be zero.
	storageFolders := smt.sm.StorageFolders()
	if len(storageFolders) != 0 {
		t.Error("storage folder was added to the storage manager despite a dependency failure")
	}
}
*/
