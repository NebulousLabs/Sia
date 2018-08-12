package contractmanager

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
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
	// Check that the storage folder was properly eliminated.
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
	root, data := randSector()
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
		t.Error("metadata file should have been removed")
	}
	_, err = os.Stat(filepath.Join(storageFolderDir, sectorFile))
	if !os.IsNotExist(err) {
		t.Error("sector file should have been removed")
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

// TestRemoveStorageFolderConcurrentAddSector will try removing a storage
// folder at the same time that sectors are being added to the contract
// manager.
func TestRemoveStorageFolderConcurrentAddSector(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester("TestRemoveStorageFolderConcurrentAddSector")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add three storage folders.
	storageFolderOne := filepath.Join(cmt.persistDir, "storageFolderOne")
	storageFolderTwo := filepath.Join(cmt.persistDir, "storageFolderTwo")
	storageFolderThree := filepath.Join(cmt.persistDir, "storageFolderThree")
	storageFolderFour := filepath.Join(cmt.persistDir, "storageFolderFour")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderOne, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderOne, modules.SectorSize*storageFolderGranularity)
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll(storageFolderTwo, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderTwo, modules.SectorSize*storageFolderGranularity*15)
	if err != nil {
		t.Fatal(err)
	}
	sfs := cmt.cm.StorageFolders()
	err = os.MkdirAll(storageFolderThree, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderThree, modules.SectorSize*storageFolderGranularity*25)
	if err != nil {
		t.Fatal(err)
	}

	// Run a goroutine that will continually add sectors to the contract
	// manager.
	var sliceLock sync.Mutex
	var roots []crypto.Hash
	var datas [][]byte
	adderTerminator := make(chan struct{})
	var adderWG sync.WaitGroup
	// Spin up 250 of these threads, putting load on the disk and increasing the
	// change of complications.
	for i := 0; i < 100; i++ {
		adderWG.Add(1)
		go func() {
			for {
				root, data := randSector()
				err := cmt.cm.AddSector(root, data)
				if err != nil {
					t.Error(err)
				}
				sliceLock.Lock()
				roots = append(roots, root)
				datas = append(datas, data)
				sliceLock.Unlock()

				// See if we are done.
				select {
				case <-adderTerminator:
					adderWG.Done()
					return
				default:
					continue
				}
			}
		}()
	}

	// Add a fourth storage folder, mostly because it takes time and guarantees
	// that a bunch of sectors will be added to the disk.
	err = os.MkdirAll(storageFolderFour, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderFour, modules.SectorSize*storageFolderGranularity*50)
	if err != nil {
		t.Fatal(err)
	}

	// In two separate goroutines, remove storage folders one and two.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		err := cmt.cm.RemoveStorageFolder(sfs[0].Index, false)
		if err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer wg.Done()
		err := cmt.cm.RemoveStorageFolder(sfs[1].Index, false)
		if err != nil {
			t.Error(err)
		}
	}()
	wg.Wait()

	// Copy over the sectors that have been added thus far.
	sliceLock.Lock()
	addedRoots := make([]crypto.Hash, len(roots))
	addedDatas := make([][]byte, len(datas))
	copy(addedRoots, roots)
	copy(addedDatas, datas)
	sliceLock.Unlock()

	// Read all of the sectors to verify that consistency is being maintained.
	for i, root := range addedRoots {
		data, err := cmt.cm.ReadSector(root)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, addedDatas[i]) {
			t.Error("Retrieved data does not match the intended data")
		}
	}

	// Close the adder threads and wait until all goroutines have finished up.
	close(adderTerminator)
	adderWG.Wait()

	// Count the number of sectors total.
	sfs = cmt.cm.StorageFolders()
	var totalConsumed uint64
	for _, sf := range sfs {
		totalConsumed = totalConsumed + (sf.Capacity - sf.CapacityRemaining)
	}
	if totalConsumed != uint64(len(roots))*modules.SectorSize {
		t.Error("Wrong storage folder consumption being reported.")
	}

	// Make sure that each sector is retreivable.
	for i, root := range roots {
		data, err := cmt.cm.ReadSector(root)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, datas[i]) {
			t.Error("Retrieved data does not match the intended data")
		}
	}

	// Restart the contract manager and verify that the changes stuck.
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

	// Count the number of sectors total.
	sfs = cmt.cm.StorageFolders()
	totalConsumed = 0
	for _, sf := range sfs {
		totalConsumed = totalConsumed + (sf.Capacity - sf.CapacityRemaining)
	}
	if totalConsumed != uint64(len(roots))*modules.SectorSize {
		t.Error("Wrong storage folder consumption being reported.")
	}

	// Make sure that each sector is retreivable.
	for i, root := range roots {
		data, err := cmt.cm.ReadSector(root)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, datas[i]) {
			t.Error("Retrieved data does not match the intended data")
		}
	}
}
