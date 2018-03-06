package contractmanager

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

// dependencyNoRecheck prevents the recheck loop from running in the contract
// manager.
type dependencyNoRecheck struct {
	modules.ProductionDependencies
}

// disrupt prevents the recheck loop from running in the contract manager.
func (*dependencyNoRecheck) Disrupt(s string) bool {
	if s == "noRecheck" {
		return true
	}
	return false
}

// TestLoadMissingStorageFolder checks that loading a storage folder which is
// missing doesn't result in a complete loss of the storage folder on subsequent
// startups.
func TestLoadMissingStorageFolder(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester(t.Name())
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

	// Check that the storage folder has been added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder reported")
	}
	// Check that the storage folder has the right path and size.
	if sfs[0].Path != storageFolderDir {
		t.Error("storage folder reported with wrong path")
	}
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("storage folder reported with wrong sector size")
	}

	// Add a sector to the storage folder.
	root, data := randSector()
	err = cmt.cm.AddSector(root, data)
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
	sfOneIndex := sfs[0].Index

	// Try reloading the contract manager after the storage folder has been
	// moved somewhere else.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	// Move the storage folder directory to a new location - hiding it from the
	// contract manager.
	err = os.Rename(storageFolderDir, storageFolderDir+"-moved")
	if err != nil {
		t.Fatal(err)
	}
	// Re-open the contract manager.
	d := new(dependencyNoRecheck)
	cmt.cm, err = newContractManager(d, filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}
	// The contract manager should still be reporting the storage folder, but
	// with errors reported.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("wrong number of storage folders being reported")
	}
	if sfs[0].FailedReads < 100000000 {
		t.Error("Not enough failures reported for absent storage folder")
	}
	if sfs[0].FailedWrites < 100000000 {
		t.Error("Not enough failures reported for absent storage folder")
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity, sfs[0].CapacityRemaining)
	}

	// Reload the contract manager and make sure the storage folder is still
	// there.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	// Re-open the contract manager.
	cmt.cm, err = newContractManager(d, filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}
	// The contract manager should still be reporting the storage folder with
	// errors.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("wrong number of storage folders being reported")
	}
	if sfs[0].FailedReads < 100000000 {
		t.Error("Not enough failures reported for absent storage folder")
	}
	if sfs[0].FailedWrites < 100000000 {
		t.Error("Not enough failures reported for absent storage folder")
	}

	// Try reading the sector from the missing storage folder.
	_, err = cmt.cm.ReadSector(root)
	if err == nil {
		t.Fatal("Expecting error when reading missing sector.")
	}

	// Try adding a sector to the contract manager - no folder can receive it.
	rootF, dataF := randSector()
	err = cmt.cm.AddSector(rootF, dataF)
	if err == nil {
		t.Error("should not be able to add sector")
	}

	// Check that you can add folders, add sectors while the contract manager
	// correctly works around the missing storage folder.
	storageFolderTwo := filepath.Join(cmt.persistDir, "storageFolderTwo")
	err = os.MkdirAll(storageFolderTwo, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderTwo, modules.SectorSize*storageFolderGranularity*2)
	if err != nil {
		t.Fatal(err)
	}
	// Add a sector to the storage folder.
	root2, data2 := randSector()
	err = cmt.cm.AddSector(root2, data2)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the sector was successfully added.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 2 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	for i := range sfs {
		if sfs[i].Capacity != sfs[i].CapacityRemaining+modules.SectorSize {
			t.Error("One sector's worth of capacity should be consumed:", sfs[i].Capacity, sfs[i].CapacityRemaining, sfs[i].Path)
		}
	}
	var sfTwoIndex uint16
	if sfs[0].Index == sfOneIndex {
		sfTwoIndex = sfs[1].Index
	} else {
		sfTwoIndex = sfs[0].Index
	}

	// Add two more sectors.
	root3, data3 := randSector()
	err = cmt.cm.AddSector(root3, data3)
	if err != nil {
		t.Fatal(err)
	}
	root4, data4 := randSector()
	err = cmt.cm.AddSector(root4, data4)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the sector was successfully added.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 2 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize*3 && sfs[1].Capacity != sfs[1].CapacityRemaining+modules.SectorSize*3 {
		t.Error("One sector's worth of capacity should be consumed")
	}

	// Try to shrink the missing storage folder.
	err = cmt.cm.ResizeStorageFolder(sfOneIndex, modules.SectorSize*storageFolderGranularity, false)
	if err == nil {
		t.Fatal("should not be able to resize a missing storage folder")
	}
	err = cmt.cm.ResizeStorageFolder(sfOneIndex, modules.SectorSize*storageFolderGranularity, true)
	if err == nil {
		t.Fatal("should not be able to resize a missing storage folder")
	}

	// Check that the storage folder is still the original size.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 2 {
		t.Fatal("wrong storage folder count")
	}
	if sfs[0].Index == sfOneIndex && sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("Storage folder has wrong size after failing to resize")
	}
	if sfs[1].Index == sfOneIndex && sfs[1].Capacity != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("Storage folder has wrong size after failing to resize")
	}

	// Try to grow the missing storage folder.
	err = cmt.cm.ResizeStorageFolder(sfOneIndex, modules.SectorSize*storageFolderGranularity*4, false)
	if err == nil {
		t.Fatal("should not be able to resize a missing storage folder")
	}
	err = cmt.cm.ResizeStorageFolder(sfOneIndex, modules.SectorSize*storageFolderGranularity*4, true)
	if err == nil {
		t.Fatal("should not be able to resize a missing storage folder")
	}

	// Check that the storage folder is still the original size.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 2 {
		t.Fatal("wrong storage folder count")
	}
	if sfs[0].Index == sfOneIndex && sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("Storage folder has wrong size after failing to resize")
	}
	if sfs[1].Index == sfOneIndex && sfs[1].Capacity != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("Storage folder has wrong size after failing to resize")
	}

	// Check that you can delete sectors and have the contract manager work
	// correctly around the missing storage folder.
	err = cmt.cm.DeleteSector(root2)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.DeleteSector(root3)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.DeleteSector(root4)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the sectors are no longer reported.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 2 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining && sfs[1].Capacity != sfs[1].CapacityRemaining {
		t.Error("Deleted sector does not seem to have been deleted correctly.")
	}
	// Try reading the deleted sector.
	_, err = cmt.cm.ReadSector(root2)
	if err == nil {
		t.Fatal("should get an error when reading a deleted sector")
	}

	// Check that it's okay to shrink a storage folder while missing a storage
	// folder.
	//
	// Start by resizing the second storage folder so it can hold a lot of
	// sectors.
	err = cmt.cm.ResizeStorageFolder(sfTwoIndex, modules.SectorSize*storageFolderGranularity*4, false)
	if err != nil {
		t.Fatal(err)
	}
	// Add enough sectors to the storage folder that doing a shrink operation
	// causes sectors to be moved around.
	num := int(storageFolderGranularity*3 + 2)
	roots := make([]crypto.Hash, num)
	datas := make([][]byte, num)
	var wg sync.WaitGroup // Add in parallel to get massive performance boost.
	for i := 0; i < num; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rootI, dataI := randSector()
			roots[i] = rootI
			datas[i] = dataI
			err := cmt.cm.AddSector(rootI, dataI)
			if err != nil {
				t.Fatal(err)
			}
		}(i)
	}
	wg.Wait()
	// Make a new storage folder so the sectors have somewhere to go.
	storageFolderThree := filepath.Join(cmt.persistDir, "storageFolderThree")
	err = os.MkdirAll(storageFolderThree, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderThree, modules.SectorSize*storageFolderGranularity)
	if err != nil {
		t.Fatal(err)
	}
	// Shrink the second storage folder such that some of the sectors are forced
	// to move.
	err = cmt.cm.ResizeStorageFolder(sfTwoIndex, modules.SectorSize*storageFolderGranularity*3, false)
	if err != nil {
		t.Fatal(err)
	}
	// Check that all of the sectors are still recoverable.
	for i := range roots {
		data, err := cmt.cm.ReadSector(roots[i])
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, datas[i]) {
			t.Error("read sector does not have the same data that was inserted")
		}
	}

	// Shrink the second storage folder again, such that there is not enough
	// room in the available storage folders to accept the data.
	err = cmt.cm.ResizeStorageFolder(sfTwoIndex, modules.SectorSize*storageFolderGranularity*2, false)
	if err == nil {
		t.Fatal("expected an error")
	}
	// Check that all of the sectors are still recoverable.
	for i := range roots {
		data, err := cmt.cm.ReadSector(roots[i])
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, datas[i]) {
			t.Error("read sector does not have the same data that was inserted")
		}
	}

	// Shrink the second storage folder again, such that there is not enough
	// room in the available storage folders to accept the data.
	err = cmt.cm.ResizeStorageFolder(sfTwoIndex, modules.SectorSize*storageFolderGranularity, true)
	if err != nil {
		t.Fatal(err)
	}
	// There is now data loss.

	// Try deleting the second storage folder, which again will cause data loss.
	err = cmt.cm.RemoveStorageFolder(sfTwoIndex, false)
	if err == nil {
		t.Fatal("should have gotten an error when trying to remove the storage folder.")
	}
	err = cmt.cm.RemoveStorageFolder(sfTwoIndex, true)
	if err != nil {
		t.Fatal(err)
	}

	// Try to recover the missing storage folder by closing and moving the
	// storage folder to the right place.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = os.Rename(storageFolderDir+"-moved", storageFolderDir)
	if err != nil {
		t.Fatal(err)
	}
	// Re-open the contract manager.
	cmt.cm, err = newContractManager(d, filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}
	// The contract manager should still be reporting the storage folder, but
	// with errors reported.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 2 {
		t.Fatal("wrong number of storage folders being reported")
	}
	var sfOne modules.StorageFolderMetadata
	for _, sf := range sfs {
		if sf.Index == sfOneIndex {
			sfOne = sf
		}
	}
	if sfOne.FailedReads > 0 {
		t.Error("folder should be visible again")
	}
	if sfOne.FailedWrites > 0 {
		t.Error("folder should be visible again")
	}
	if sfOne.Capacity != sfOne.CapacityRemaining+modules.SectorSize {
		cmt.cm.wal.mu.Lock()
		t.Log("Usage len:", len(cmt.cm.storageFolders[sfOne.Index].usage))
		t.Log("Reported Sectors:", cmt.cm.storageFolders[sfOne.Index].sectors)
		t.Log("Avail:", len(cmt.cm.storageFolders[sfOne.Index].availableSectors))
		cmt.cm.wal.mu.Unlock()
		t.Error("One sector's worth of capacity should be consumed:", sfOne.Capacity, sfOne.CapacityRemaining)
	}

	// See if the sector is still available.
	recoveredData, err := cmt.cm.ReadSector(root)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(recoveredData, data) {
		t.Error("recovered data is not equal to original data")
	}

	// Redo the storage folder move, so we can test deleting a missing storage
	// folder.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	// Move the storage folder directory to a new location - hiding it from the
	// contract manager.
	err = os.Rename(storageFolderDir, storageFolderDir+"-moved")
	if err != nil {
		t.Fatal(err)
	}
	// Re-open the contract manager.
	cmt.cm, err = newContractManager(d, filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Try removing the storage folder without the --force option. It should
	// fail.
	err = cmt.cm.RemoveStorageFolder(sfOneIndex, false)
	if err == nil {
		t.Fatal("should have gotten an error")
	}
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 2 {
		t.Error("there should be two storage folders after a removal failed.")
	}
	err = cmt.cm.RemoveStorageFolder(sfOneIndex, true)
	if err != nil {
		t.Fatal(err)
	}
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Error("there should be only one storage folder remaining")
	}

	// Close and re-open the contract maanger, storage folder should still be
	// missing.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	// Re-open the contract manager.
	cmt.cm, err = newContractManager(d, filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Error("there should be only one storage folder remaining")
	}
}

// TestFolderRechecker verifies that the folder rechecker is able to discover
// when a storage folder has become available again.
func TestFolderRechecker(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester(t.Name())
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

	// Check that the storage folder has been added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder reported")
	}
	// Check that the storage folder has the right path and size.
	if sfs[0].Path != storageFolderDir {
		t.Error("storage folder reported with wrong path")
	}
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("storage folder reported with wrong sector size")
	}

	// Add a sector to the storage folder.
	root, data := randSector()
	err = cmt.cm.AddSector(root, data)
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

	// Try reloading the contract manager after the storage folder has been
	// moved somewhere else.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	// Move the storage folder directory to a new location - hiding it from the
	// contract manager.
	err = os.Rename(storageFolderDir, storageFolderDir+"-moved")
	if err != nil {
		t.Fatal(err)
	}
	// Re-open the contract manager.
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}
	// The contract manager should still be reporting the storage folder, but
	// with errors reported.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("wrong number of storage folders being reported")
	}
	if sfs[0].FailedReads < 100000000 {
		t.Error("Not enough failures reported for absent storage folder")
	}
	if sfs[0].FailedWrites < 100000000 {
		t.Error("Not enough failures reported for absent storage folder")
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity, sfs[0].CapacityRemaining)
	}

	// Move the storage folder back to where the contract manager can see it.
	err = os.Rename(storageFolderDir+"-moved", storageFolderDir)
	if err != nil {
		t.Fatal(err)
	}

	// Sleep until the rechecker can find the storage folder.
	time.Sleep(maxFolderRecheckInterval)

	// Check that the storage folder has been found by the rechecker.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("wrong number of storage folders being reported")
	}
	if sfs[0].FailedReads != 0 {
		t.Error("Not enough failures reported for absent storage folder")
	}
	if sfs[0].FailedWrites != 0 {
		t.Error("Not enough failures reported for absent storage folder")
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity, sfs[0].CapacityRemaining)
	}

	// Check that the sector is once again available.
	recoveredData, err := cmt.cm.ReadSector(root)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(recoveredData, data) {
		t.Error("recovered data does not equal original data")
	}

	// Try adding a sector to the contract manager - no folder can receive it.
	root2, data2 := randSector()
	err = cmt.cm.AddSector(root2, data2)
	if err != nil {
		t.Error("should not be able to add sector")
	}
	recoveredData, err = cmt.cm.ReadSector(root2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(recoveredData, data2) {
		t.Error("recovered data does not equal original data")
	}

	// Grow the storage folder.
	err = cmt.cm.ResizeStorageFolder(sfs[0].Index, modules.SectorSize*storageFolderGranularity*4, false)
	if err != nil {
		t.Fatal(err)
	}
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("wrong number of storage folders being reported")
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize*2 {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity, sfs[0].CapacityRemaining)
	}
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*4 {
		t.Error("the storage folder growth does not seem to have worked")
	}

	// Restart the client. Sector should still be readable, storage folder
	// should still be grown.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}
	// Check that the sector is once again available.
	recoveredData, err = cmt.cm.ReadSector(root)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(recoveredData, data) {
		t.Error("recovered data does not equal original data")
	}
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("wrong number of storage folders being reported")
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize*2 {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity, sfs[0].CapacityRemaining)
	}
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*4 {
		t.Error("the storage folder growth does not seem to have worked")
	}
}
