package contractmanager

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// TestRemoveStorageFolder tries removing a storage folder that has no sectors
// in it.
func TestRemoveStorageFolder(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester("TestRemoveStorageFolder")
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
	err = cmt.cm.AddStorageFolder(storageFolderDir, modules.SectorSize*storageFolderGranularity*2)
	if err != nil {
		t.Fatal(err)
	}

	// Determine the index of the storage folder.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("there should be storage folder in the contract manager")
	}
	err = cmt.cm.RemoveStorageFolder(sfs[0].Index, false)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the storage folder has been removed.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 0 {
		t.Fatal("Storage folder should have been removed")
	}
	// Check that the disk objects were removed.
	_, err = os.Stat(filepath.Join(storageFolderDir, metadataFile))
	if !os.IsNotExist(err) {
		t.Fatal("metadata file should have been removed")
	}
	_, err = os.Stat(filepath.Join(storageFolderDir, sectorFile))
	if !os.IsNotExist(err) {
		t.Fatal("sector file should have been removed")
	}

	// Restart the contract manager to see if the storage folder is still gone.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	// Create the new contract manager using the same persist dir, so that it
	// will see the uncommitted WAL.
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}
	// Check that the storage folder was properly recovered.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 0 {
		t.Fatal("Storage folder should have been removed")
	}
}

// TestRemoveStorageFolderWithSector tries removing a storage folder that has a
// sector in it.
func TestRemoveStorageFolderWithSector(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester("TestRemoveStorageFolderWithSector")
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
	err = cmt.cm.AddStorageFolder(storageFolderDir, modules.SectorSize*storageFolderGranularity*2)
	if err != nil {
		t.Fatal(err)
	}
	// Give the storage folder a sector.
	root, data, err := randSector()
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddSector(root, data)
	if err != nil {
		t.Fatal(err)
	}

	// Determine the index of the storage folder.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("there should be storage folder in the contract manager")
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Fatal("there should be one sector reported in the storage folder")
	}
	// Verify that the data held within the storage folder is the correct data.
	readData, err := cmt.cm.ReadSector(root)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readData, data) {
		t.Fatal("Reading a sector from the storage folder did not produce the right data")
	}

	// Add a second storage folder, then remove the first storage folder.
	storageFolderTwo := filepath.Join(cmt.persistDir, "storageFolderTwo")
	err = os.MkdirAll(storageFolderTwo, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderTwo, modules.SectorSize*storageFolderGranularity*2)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.RemoveStorageFolder(sfs[0].Index, false)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the remaining storage folder has picked up the right sector.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("there should be storage folder in the contract manager")
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Fatal("there should be one sector reported in the storage folder")
	}
	// Verify that the data held within the storage folder is the correct data.
	readData, err = cmt.cm.ReadSector(root)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readData, data) {
		t.Fatal("Reading a sector from the storage folder did not produce the right data")
	}

	// Check that the disk objects were removed.
	_, err = os.Stat(filepath.Join(storageFolderDir, metadataFile))
	if !os.IsNotExist(err) {
		t.Fatal("metadata file should have been removed")
	}
	_, err = os.Stat(filepath.Join(storageFolderDir, sectorFile))
	if !os.IsNotExist(err) {
		t.Fatal("sector file should have been removed")
	}

	// Restart the contract manager to see if the storage folder is still gone.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	// Create the new contract manager using the same persist dir, so that it
	// will see the uncommitted WAL.
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("there should be storage folder in the contract manager")
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Fatal("there should be one sector reported in the storage folder")
	}
	// Verify that the data held within the storage folder is the correct data.
	readData, err = cmt.cm.ReadSector(root)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readData, data) {
		t.Fatal("Reading a sector from the storage folder did not produce the right data")
	}
}
