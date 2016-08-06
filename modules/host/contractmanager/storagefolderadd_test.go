package contractmanager

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// TestAddStorageFolder tries to add a storage folder to the contract manager,
// blocking until the add has completed.
func TestAddStorageFolder(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cmt, err := newContractManagerTester("TestAddStorageFolder")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder to the contract manager tester.
	storageFolderDir := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderDir, 0700)
	err = cmt.cm.AddStorageFolder(storageFolderDir, modules.SectorSize*32*2)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the storage folder has been added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder reported")
	}
	// Check that the storage folder has the right path and size.
	if sfs[0].Path != storageFolderDir {
		t.Error("storage folder reported with wrong path")
	}
	if sfs[0].Capacity != modules.SectorSize*32*2 {
		t.Error("storage folder reported with wrong sector size")
	}
}
