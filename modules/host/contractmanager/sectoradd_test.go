package contractmanager

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// TestAddSector tries to add a sector to the contract manager, blocking until
// the add has completed.
func TestAddSector(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester("TestAddSector")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder to the contract manager tester.
	storageFolderDir := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderDir, modules.SectorSize*64)
	if err != nil {
		t.Fatal(err)
	}

	// Fabricate a sector and add it to the contract manager.
	root, data, err := randSector()
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddSector(root, data)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the sector was successfully added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity, sfs[0].CapacityRemaining)
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the Usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 1 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 1 {
		t.Fatal("storage folder not being reported correctly")
	}
	var index uint16
	for _, sf := range cmt.cm.storageFolders {
		index = sf.Index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.count != 1 {
			t.Error("Sector location should only be reporting one sector")
		}
		if sl.storageFolder != index {
			t.Error("Sector location is being reported incorrectly - wrong storage folder")
		}
		if sl.index > 64 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}

	// Try reloading the contract manager and see if all of the stateful checks
	// still hold.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the sector was successfully added.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity, sfs[0].CapacityRemaining)
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the Usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 1 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 1 {
		t.Fatal("storage folder not being reported correctly")
	}
	for _, sf := range cmt.cm.storageFolders {
		index = sf.Index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.count != 1 {
			t.Error("Sector location should only be reporting one sector", sl.count)
		}
		if sl.storageFolder != index {
			t.Error("Sector location is being reported incorrectly - wrong storage folder", sl.storageFolder, index)
		}
		if sl.index > 64 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}
}
