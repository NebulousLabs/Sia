package contractmanager

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"gitlab.com/NebulousLabs/Sia/crypto"
	"gitlab.com/NebulousLabs/Sia/modules"
)

// TestShrinkStorageFolder checks that a storage folder can be successfully
// decreased in size.
func TestShrinkStorageFolder(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester("TestShrinkStorageFolder")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder.
	storageFolderOne := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderOne, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderOne, modules.SectorSize*storageFolderGranularity*8)
	if err != nil {
		t.Fatal(err)
	}

	// Get the index of the storage folder.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("there should only be one storage folder")
	}
	sfIndex := sfs[0].Index
	// Verify that the storage folder has the correct capacity.
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*8 {
		t.Error("new storage folder is reporting the wrong capacity")
	}
	// Verify that the on-disk files are the right size.
	mfn := filepath.Join(storageFolderOne, metadataFile)
	sfn := filepath.Join(storageFolderOne, sectorFile)
	mfi, err := os.Stat(mfn)
	if err != nil {
		t.Fatal(err)
	}
	sfi, err := os.Stat(sfn)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*8 {
		t.Error("metadata file is the wrong size")
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*8 {
		t.Error("sector file is the wrong size")
	}

	// Decrease the size of the storage folder.
	err = cmt.cm.ResizeStorageFolder(sfIndex, modules.SectorSize*storageFolderGranularity*2, false)
	if err != nil {
		t.Fatal(err)
	}
	// Verify that the capacity and file sizes are correct.
	sfs = cmt.cm.StorageFolders()
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("new storage folder is reporting the wrong capacity")
	}
	mfi, err = os.Stat(mfn)
	if err != nil {
		t.Fatal(err)
	}
	sfi, err = os.Stat(sfn)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*2 {
		t.Error("metadata file is the wrong size")
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("sector file is the wrong size")
	}

	// Restart the contract manager to see that the change is persistent.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the capacity and file sizes are correct.
	sfs = cmt.cm.StorageFolders()
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("new storage folder is reporting the wrong capacity")
	}
	mfi, err = os.Stat(mfn)
	if err != nil {
		t.Fatal(err)
	}
	sfi, err = os.Stat(sfn)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*2 {
		t.Error("metadata file is the wrong size")
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("sector file is the wrong size")
	}
}

// TestShrinkStorageFolderWithSectors checks that a storage folder can be
// successfully decreased in size when it has sectors which would need to be
// moved.
func TestShrinkStorageFolderWithSectors(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder.
	storageFolderOne := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderOne, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderOne, modules.SectorSize*storageFolderGranularity*8)
	if err != nil {
		t.Fatal(err)
	}

	// Get the index of the storage folder.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("there should only be one storage folder")
	}
	sfIndex := sfs[0].Index
	// Verify that the storage folder has the correct capacity.
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*8 {
		t.Error("new storage folder is reporting the wrong capacity")
	}
	// Verify that the on-disk files are the right size.
	mfn := filepath.Join(storageFolderOne, metadataFile)
	sfn := filepath.Join(storageFolderOne, sectorFile)
	mfi, err := os.Stat(mfn)
	if err != nil {
		t.Fatal(err)
	}
	sfi, err := os.Stat(sfn)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*8 {
		t.Error("metadata file is the wrong size")
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*8 {
		t.Error("sector file is the wrong size")
	}

	// Create some sectors and add them to the storage folder.
	roots := make([]crypto.Hash, storageFolderGranularity*3)
	datas := make([][]byte, storageFolderGranularity*3)
	for i := 0; i < storageFolderGranularity*3; i++ {
		root, data := randSector()
		roots[i] = root
		datas[i] = data
	}
	// Add all of the sectors.
	var wg sync.WaitGroup
	wg.Add(len(roots))
	for i := 0; i < len(roots); i++ {
		go func(i int) {
			err := cmt.cm.AddSector(roots[i], datas[i])
			if err != nil {
				t.Error(err)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()

	// Add a second storage folder so that the displaced sectors have somewhere
	// to go.
	storageFolderTwo := filepath.Join(cmt.persistDir, "storageFolderTwo")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderTwo, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderTwo, modules.SectorSize*storageFolderGranularity*3)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that every single sector is readable and has the correct data.
	wg.Add(len(roots))
	var misses uint64
	for i := 0; i < len(roots); i++ {
		go func(i int) {
			data, err := cmt.cm.ReadSector(roots[i])
			if err != nil || !bytes.Equal(data, datas[i]) {
				atomic.AddUint64(&misses, 1)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
	if misses != 0 {
		t.Errorf("Could not find all %v sectors: %v\n", len(roots), misses)
	}

	// Decrease the size of the storage folder.
	err = cmt.cm.ResizeStorageFolder(sfIndex, modules.SectorSize*storageFolderGranularity*2, false)
	if err != nil {
		t.Fatal(err)
	}
	// Verify that the capacity and file sizes are correct.
	sfs = cmt.cm.StorageFolders()
	capacity := sfs[0].Capacity + sfs[1].Capacity
	capacityRemaining := sfs[0].CapacityRemaining + sfs[1].CapacityRemaining
	if capacity != modules.SectorSize*storageFolderGranularity*5 {
		t.Error("new storage folder is reporting the wrong capacity")
	}
	if capacityRemaining != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("new storage folder capacity remaining is reporting the wrong remaining capacity")
	}
	mfi, err = os.Stat(mfn)
	if err != nil {
		t.Fatal(err)
	}
	sfi, err = os.Stat(sfn)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*2 {
		t.Error("metadata file is the wrong size")
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("sector file is the wrong size")
	}

	// Verify that every single sector is readable and has the correct data.
	wg.Add(len(roots))
	misses = 0
	for i := 0; i < len(roots); i++ {
		go func(i int) {
			data, err := cmt.cm.ReadSector(roots[i])
			if err != nil || !bytes.Equal(data, datas[i]) {
				atomic.AddUint64(&misses, 1)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
	if misses != 0 {
		t.Errorf("Could not find all %v sectors: %v\n", len(roots), misses)
	}

	// Restart the contract manager to see that the change is persistent.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the capacity and file sizes are correct.
	sfs = cmt.cm.StorageFolders()
	capacity = sfs[0].Capacity + sfs[1].Capacity
	capacityRemaining = sfs[0].CapacityRemaining + sfs[1].CapacityRemaining
	if capacity != modules.SectorSize*storageFolderGranularity*5 {
		t.Error("new storage folder is reporting the wrong capacity")
	}
	if capacityRemaining != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("new storage folder capacity remaining is reporting the wrong remaining capacity")
	}
	mfi, err = os.Stat(mfn)
	if err != nil {
		t.Fatal(err)
	}
	sfi, err = os.Stat(sfn)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*2 {
		t.Error("metadata file is the wrong size")
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("sector file is the wrong size")
	}

	// Verify that every single sector is readable and has the correct data.
	wg.Add(len(roots))
	misses = 0
	for i := 0; i < len(roots); i++ {
		go func(i int) {
			data, err := cmt.cm.ReadSector(roots[i])
			if err != nil || !bytes.Equal(data, datas[i]) {
				atomic.AddUint64(&misses, 1)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
	if misses != 0 {
		t.Errorf("Could not find all %v sectors: %v\n", len(roots), misses)
	}
}

// TestShrinkStorageFolderIncopmleteWrite checks that shrinkStorageFolder
// operates as intended when the writing to move sectors cannot complete fully.
func TestShrinkStorageFolderIncompleteWrite(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	d := new(dependencyIncompleteGrow)
	cmt, err := newMockedContractManagerTester(d, "TestShrinkStorageFolderIncompleteWrite")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder.
	storageFolderOne := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderOne, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderOne, modules.SectorSize*storageFolderGranularity*8)
	if err != nil {
		t.Fatal(err)
	}

	// Get the index of the storage folder.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("there should only be one storage folder")
	}
	sfIndex := sfs[0].Index

	// Create some sectors and add them to the storage folder.
	roots := make([]crypto.Hash, storageFolderGranularity*3)
	datas := make([][]byte, storageFolderGranularity*3)
	for i := 0; i < storageFolderGranularity*3; i++ {
		root, data := randSector()
		roots[i] = root
		datas[i] = data
	}
	// Add all of the sectors.
	var wg sync.WaitGroup
	wg.Add(len(roots))
	for i := 0; i < len(roots); i++ {
		go func(i int) {
			err := cmt.cm.AddSector(roots[i], datas[i])
			if err != nil {
				t.Error(err)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()

	// Add a second storage folder so that the displaced sectors have somewhere
	// to go.
	storageFolderTwo := filepath.Join(cmt.persistDir, "storageFolderTwo")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderTwo, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderTwo, modules.SectorSize*storageFolderGranularity*3)
	if err != nil {
		t.Fatal(err)
	}

	// Trigger some failures.
	d.mu.Lock()
	d.threshold = 1 << 15
	d.triggered = true
	d.mu.Unlock()

	// Decrease the size of the storage folder.
	err = cmt.cm.ResizeStorageFolder(sfIndex, modules.SectorSize*storageFolderGranularity*2, false)
	if err == nil {
		t.Fatal("expected a failure")
	}
	// Verify that the capacity and file sizes are correct.
	sfs = cmt.cm.StorageFolders()
	capacity := sfs[0].Capacity + sfs[1].Capacity
	capacityRemaining := sfs[0].CapacityRemaining + sfs[1].CapacityRemaining
	if capacity != modules.SectorSize*storageFolderGranularity*11 {
		t.Error("new storage folder is reporting the wrong capacity")
	}
	if capacityRemaining != modules.SectorSize*storageFolderGranularity*8 {
		t.Error("new storage folder capacity remaining is reporting the wrong remaining capacity")
	}
	mfn := filepath.Join(storageFolderOne, metadataFile)
	sfn := filepath.Join(storageFolderOne, sectorFile)
	mfi, err := os.Stat(mfn)
	if err != nil {
		t.Fatal(err)
	}
	sfi, err := os.Stat(sfn)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*8 {
		t.Error("metadata file is the wrong size")
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*8 {
		t.Error("sector file is the wrong size")
	}

	// Verify that every single sector is readable and has the correct data.
	wg.Add(len(roots))
	var misses uint64
	for i := 0; i < len(roots); i++ {
		go func(i int) {
			data, err := cmt.cm.ReadSector(roots[i])
			if err != nil || !bytes.Equal(data, datas[i]) {
				atomic.AddUint64(&misses, 1)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
	if misses != 0 {
		t.Errorf("Could not find all %v sectors: %v\n", len(roots), misses)
	}

	// Restart the contract manager to see that the change is persistent.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the capacity and file sizes are correct.
	sfs = cmt.cm.StorageFolders()
	capacity = sfs[0].Capacity + sfs[1].Capacity
	capacityRemaining = sfs[0].CapacityRemaining + sfs[1].CapacityRemaining
	if capacity != modules.SectorSize*storageFolderGranularity*11 {
		t.Error("new storage folder is reporting the wrong capacity")
	}
	if capacityRemaining != modules.SectorSize*storageFolderGranularity*8 {
		t.Error("new storage folder capacity remaining is reporting the wrong remaining capacity")
	}
	mfi, err = os.Stat(mfn)
	if err != nil {
		t.Fatal(err)
	}
	sfi, err = os.Stat(sfn)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*8 {
		t.Error("metadata file is the wrong size")
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*8 {
		t.Error("sector file is the wrong size")
	}

	// Verify that every single sector is readable and has the correct data.
	wg.Add(len(roots))
	misses = 0
	for i := 0; i < len(roots); i++ {
		go func(i int) {
			data, err := cmt.cm.ReadSector(roots[i])
			if err != nil || !bytes.Equal(data, datas[i]) {
				atomic.AddUint64(&misses, 1)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
	if misses != 0 {
		t.Errorf("Could not find all %v sectors: %v\n", len(roots), misses)
	}
}

// TestShrinkStorageFolderIncopmleteWriteForce checks that shrinkStorageFolder
// operates as intended when the writing to move sectors cannot complete fully,
// but the 'force' flag is set.
// capacity and capacity remaining.
func TestShrinkStorageFolderIncompleteWriteForce(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	d := new(dependencyIncompleteGrow)
	cmt, err := newMockedContractManagerTester(d, "TestShrinkStorageFolderIncompleteWriteForce")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder.
	storageFolderOne := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderOne, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderOne, modules.SectorSize*storageFolderGranularity*8)
	if err != nil {
		t.Fatal(err)
	}

	// Get the index of the storage folder.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("there should only be one storage folder")
	}
	sfIndex := sfs[0].Index

	// Create some sectors and add them to the storage folder.
	roots := make([]crypto.Hash, 6)
	datas := make([][]byte, 6)
	for i := 0; i < len(roots); i++ {
		root, data := randSector()
		roots[i] = root
		datas[i] = data
	}
	// Add all of the sectors.
	var wg sync.WaitGroup
	wg.Add(len(roots))
	for i := 0; i < len(roots); i++ {
		go func(i int) {
			err := cmt.cm.AddSector(roots[i], datas[i])
			if err != nil {
				t.Error(err)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()

	// Add a second storage folder so that the displaced sectors have somewhere
	// to go.
	storageFolderTwo := filepath.Join(cmt.persistDir, "storageFolderTwo")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderTwo, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderTwo, modules.SectorSize*storageFolderGranularity*3)
	if err != nil {
		t.Fatal(err)
	}

	// Trigger some failures.
	d.mu.Lock()
	d.threshold = 1 << 11
	d.triggered = true
	d.mu.Unlock()

	// Decrease the size of the storage folder.
	err = cmt.cm.ResizeStorageFolder(sfIndex, modules.SectorSize*storageFolderGranularity*2, true)
	if err != nil {
		t.Fatal("expected a failure")
	}
	// Verify that the capacity and file sizes are correct.
	sfs = cmt.cm.StorageFolders()
	capacity := sfs[0].Capacity + sfs[1].Capacity
	capacityRemaining := sfs[0].CapacityRemaining + sfs[1].CapacityRemaining
	if capacity != modules.SectorSize*storageFolderGranularity*5 {
		t.Error("new storage folder is reporting the wrong capacity")
	}
	mfn := filepath.Join(storageFolderOne, metadataFile)
	sfn := filepath.Join(storageFolderOne, sectorFile)
	mfi, err := os.Stat(mfn)
	if err != nil {
		t.Fatal(err)
	}
	sfi, err := os.Stat(sfn)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*2 {
		t.Error("metadata file is the wrong size")
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("sector file is the wrong size")
	}

	// Data was lost. Count the number of sectors that are still available.
	wg.Add(len(roots))
	var remainingSectors uint64
	for i := 0; i < len(roots); i++ {
		go func(i int) {
			defer wg.Done()

			data, err := cmt.cm.ReadSector(roots[i])
			if err != nil {
				// Sector probably destroyed.
				return
			}
			if !bytes.Equal(data, datas[i]) {
				t.Error("ReadSector has returned the wrong data")
			}

			atomic.AddUint64(&remainingSectors, 1)
		}(i)
	}
	wg.Wait()

	// Check that the capacity remaining matches the number of reachable
	// sectors.
	if capacityRemaining != capacity-remainingSectors*modules.SectorSize {
		t.Error(capacityRemaining/modules.SectorSize, capacity/modules.SectorSize, remainingSectors)
	}

	// Restart the contract manager to see that the change is persistent.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the capacity and file sizes are correct.
	sfs = cmt.cm.StorageFolders()
	capacity = sfs[0].Capacity + sfs[1].Capacity
	capacityRemaining = sfs[0].CapacityRemaining + sfs[1].CapacityRemaining
	if capacity != modules.SectorSize*storageFolderGranularity*5 {
		t.Error("new storage folder is reporting the wrong capacity")
	}
	mfi, err = os.Stat(mfn)
	if err != nil {
		t.Fatal(err)
	}
	sfi, err = os.Stat(sfn)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*2 {
		t.Error("metadata file is the wrong size")
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("sector file is the wrong size")
	}

	// Check that the same number of sectors are still available.
	wg.Add(len(roots))
	var nowRemainingSectors uint64
	for i := 0; i < len(roots); i++ {
		go func(i int) {
			defer wg.Done()

			data, err := cmt.cm.ReadSector(roots[i])
			if err != nil {
				// Sector probably destroyed.
				return
			}
			if !bytes.Equal(data, datas[i]) {
				t.Error("ReadSector has returned the wrong data")
			}

			atomic.AddUint64(&nowRemainingSectors, 1)
		}(i)
	}
	wg.Wait()

	// Check that the capacity remaining matches the number of reachable
	// sectors.
	if capacityRemaining != capacity-remainingSectors*modules.SectorSize {
		t.Error(capacityRemaining/modules.SectorSize, capacity/modules.SectorSize, remainingSectors)
	}
	if remainingSectors != nowRemainingSectors {
		t.Error("available sector set changed after restart", remainingSectors, nowRemainingSectors)
	}
}

// dependencyShrinkNoFinalize will not add a confirmation to the WAL that a
// shrink storage folder operation has completed.
type dependencyShrinkNoFinalize struct {
	modules.ProductionDependencies
}

// disrupt will prevent the growStorageFolder operation from committing a
// finalized growStorageFolder operation to the WAL.
func (*dependencyShrinkNoFinalize) Disrupt(s string) bool {
	if s == "incompleteShrinkStorageFolder" {
		return true
	}
	if s == "cleanWALFile" {
		return true
	}
	return false
}

// TestShrinkStorageFolderShutdownAfterMove simulates an unclean shutdown that
// occurs after the storage folder sector move has completed, but before it has
// established through the WAL that the move has completed. The result should
// be that the storage folder shirnk is not accepted after restart.
func TestShrinkStorageFolderShutdownAfterMove(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	d := new(dependencyShrinkNoFinalize)
	cmt, err := newMockedContractManagerTester(d, "TestShrinkStorageFolderShutdownAfterMove")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder.
	storageFolderOne := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderOne, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderOne, modules.SectorSize*storageFolderGranularity*8)
	if err != nil {
		t.Fatal(err)
	}

	// Get the index of the storage folder.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("there should only be one storage folder")
	}
	sfIndex := sfs[0].Index
	// Verify that the storage folder has the correct capacity.
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*8 {
		t.Error("new storage folder is reporting the wrong capacity")
	}
	// Verify that the on-disk files are the right size.
	mfn := filepath.Join(storageFolderOne, metadataFile)
	sfn := filepath.Join(storageFolderOne, sectorFile)
	mfi, err := os.Stat(mfn)
	if err != nil {
		t.Fatal(err)
	}
	sfi, err := os.Stat(sfn)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*8 {
		t.Error("metadata file is the wrong size")
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*8 {
		t.Error("sector file is the wrong size")
	}

	// Create some sectors and add them to the storage folder.
	roots := make([]crypto.Hash, storageFolderGranularity*3)
	datas := make([][]byte, storageFolderGranularity*3)
	for i := 0; i < storageFolderGranularity*3; i++ {
		root, data := randSector()
		roots[i] = root
		datas[i] = data
	}
	// Add all of the sectors.
	var wg sync.WaitGroup
	wg.Add(len(roots))
	for i := 0; i < len(roots); i++ {
		go func(i int) {
			err := cmt.cm.AddSector(roots[i], datas[i])
			if err != nil {
				t.Error(err)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()

	// Add a second storage folder so that the displaced sectors have somewhere
	// to go.
	storageFolderTwo := filepath.Join(cmt.persistDir, "storageFolderTwo")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderTwo, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderTwo, modules.SectorSize*storageFolderGranularity*3)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that every single sector is readable and has the correct data.
	wg.Add(len(roots))
	var misses uint64
	for i := 0; i < len(roots); i++ {
		go func(i int) {
			data, err := cmt.cm.ReadSector(roots[i])
			if err != nil || !bytes.Equal(data, datas[i]) {
				atomic.AddUint64(&misses, 1)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
	if misses != 0 {
		t.Errorf("Could not find all %v sectors: %v\n", len(roots), misses)
	}

	// Decrease the size of the storage folder.
	err = cmt.cm.ResizeStorageFolder(sfIndex, modules.SectorSize*storageFolderGranularity*2, false)
	if err != nil {
		t.Fatal(err)
	}
	// Verify that the capacity and file sizes are correct.
	sfs = cmt.cm.StorageFolders()
	capacity := sfs[0].Capacity + sfs[1].Capacity
	capacityRemaining := sfs[0].CapacityRemaining + sfs[1].CapacityRemaining
	if capacity != modules.SectorSize*storageFolderGranularity*11 {
		t.Error("new storage folder is reporting the wrong capacity")
	}
	if capacityRemaining != modules.SectorSize*storageFolderGranularity*8 {
		t.Error("new storage folder capacity remaining is reporting the wrong remaining capacity")
	}
	mfi, err = os.Stat(mfn)
	if err != nil {
		t.Fatal(err)
	}
	sfi, err = os.Stat(sfn)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*8 {
		t.Error("metadata file is the wrong size")
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*8 {
		t.Error("sector file is the wrong size")
	}

	// Verify that every single sector is readable and has the correct data.
	wg.Add(len(roots))
	misses = 0
	for i := 0; i < len(roots); i++ {
		go func(i int) {
			data, err := cmt.cm.ReadSector(roots[i])
			if err != nil || !bytes.Equal(data, datas[i]) {
				atomic.AddUint64(&misses, 1)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
	if misses != 0 {
		t.Errorf("Could not find all %v sectors: %v\n", len(roots), misses)
	}

	// Restart the contract manager. WAL update was not completed, so changes
	// should not have persisted. All sectors should still be available though,
	// and they may have moved around but the capacity reporting should align
	// correctly.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the capacity and file sizes are correct.
	sfs = cmt.cm.StorageFolders()
	capacity = sfs[0].Capacity + sfs[1].Capacity
	capacityRemaining = sfs[0].CapacityRemaining + sfs[1].CapacityRemaining
	if capacity != modules.SectorSize*storageFolderGranularity*11 {
		t.Error("new storage folder is reporting the wrong capacity", capacity/modules.SectorSize, storageFolderGranularity*11)
	}
	if capacityRemaining != modules.SectorSize*storageFolderGranularity*8 {
		t.Error("new storage folder capacity remaining is reporting the wrong remaining capacity")
	}
	mfi, err = os.Stat(mfn)
	if err != nil {
		t.Fatal(err)
	}
	sfi, err = os.Stat(sfn)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*8 {
		t.Error("metadata file is the wrong size")
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*8 {
		t.Error("sector file is the wrong size")
	}

	// Verify that every single sector is readable and has the correct data.
	wg.Add(len(roots))
	misses = 0
	for i := 0; i < len(roots); i++ {
		go func(i int) {
			data, err := cmt.cm.ReadSector(roots[i])
			if err != nil || !bytes.Equal(data, datas[i]) {
				atomic.AddUint64(&misses, 1)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
	if misses != 0 {
		t.Errorf("Could not find all %v sectors: %v\n", len(roots), misses)
	}
}

// TestShrinkStorageFolderWAL completes a storage folder shrinking, but leaves
// the WAL behind so that a commit is necessary to finalize things.
func TestShrinkStorageFolderWAL(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	d := new(dependencyLeaveWAL)
	cmt, err := newMockedContractManagerTester(d, "TestShrinkStorageFolderWAL")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder.
	storageFolderOne := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderOne, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderOne, modules.SectorSize*storageFolderGranularity*8)
	if err != nil {
		t.Fatal(err)
	}

	// Get the index of the storage folder.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("there should only be one storage folder")
	}
	sfIndex := sfs[0].Index
	// Verify that the storage folder has the correct capacity.
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*8 {
		t.Error("new storage folder is reporting the wrong capacity")
	}
	// Verify that the on-disk files are the right size.
	mfn := filepath.Join(storageFolderOne, metadataFile)
	sfn := filepath.Join(storageFolderOne, sectorFile)
	mfi, err := os.Stat(mfn)
	if err != nil {
		t.Fatal(err)
	}
	sfi, err := os.Stat(sfn)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*8 {
		t.Error("metadata file is the wrong size")
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*8 {
		t.Error("sector file is the wrong size")
	}

	// Create some sectors and add them to the storage folder.
	roots := make([]crypto.Hash, storageFolderGranularity*3)
	datas := make([][]byte, storageFolderGranularity*3)
	for i := 0; i < storageFolderGranularity*3; i++ {
		root, data := randSector()
		roots[i] = root
		datas[i] = data
	}
	// Add all of the sectors.
	var wg sync.WaitGroup
	wg.Add(len(roots))
	for i := 0; i < len(roots); i++ {
		go func(i int) {
			err := cmt.cm.AddSector(roots[i], datas[i])
			if err != nil {
				t.Error(err)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()

	// Add a second storage folder so that the displaced sectors have somewhere
	// to go.
	storageFolderTwo := filepath.Join(cmt.persistDir, "storageFolderTwo")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderTwo, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderTwo, modules.SectorSize*storageFolderGranularity*3)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that every single sector is readable and has the correct data.
	wg.Add(len(roots))
	var misses uint64
	for i := 0; i < len(roots); i++ {
		go func(i int) {
			data, err := cmt.cm.ReadSector(roots[i])
			if err != nil || !bytes.Equal(data, datas[i]) {
				atomic.AddUint64(&misses, 1)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
	if misses != 0 {
		t.Errorf("Could not find all %v sectors: %v\n", len(roots), misses)
	}

	// Decrease the size of the storage folder.
	err = cmt.cm.ResizeStorageFolder(sfIndex, modules.SectorSize*storageFolderGranularity*2, false)
	if err != nil {
		t.Fatal(err)
	}
	// Verify that the capacity and file sizes are correct.
	sfs = cmt.cm.StorageFolders()
	capacity := sfs[0].Capacity + sfs[1].Capacity
	capacityRemaining := sfs[0].CapacityRemaining + sfs[1].CapacityRemaining
	if capacity != modules.SectorSize*storageFolderGranularity*5 {
		t.Error("new storage folder is reporting the wrong capacity")
	}
	if capacityRemaining != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("new storage folder capacity remaining is reporting the wrong remaining capacity")
	}
	mfi, err = os.Stat(mfn)
	if err != nil {
		t.Fatal(err)
	}
	sfi, err = os.Stat(sfn)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*2 {
		t.Error("metadata file is the wrong size")
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("sector file is the wrong size")
	}

	// Verify that every single sector is readable and has the correct data.
	wg.Add(len(roots))
	misses = 0
	for i := 0; i < len(roots); i++ {
		go func(i int) {
			data, err := cmt.cm.ReadSector(roots[i])
			if err != nil || !bytes.Equal(data, datas[i]) {
				atomic.AddUint64(&misses, 1)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
	if misses != 0 {
		t.Errorf("Could not find all %v sectors: %v\n", len(roots), misses)
	}

	// Restart the contract manager to see that the change is persistent.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the capacity and file sizes are correct.
	sfs = cmt.cm.StorageFolders()
	capacity = sfs[0].Capacity + sfs[1].Capacity
	capacityRemaining = sfs[0].CapacityRemaining + sfs[1].CapacityRemaining
	if capacity != modules.SectorSize*storageFolderGranularity*5 {
		t.Error("new storage folder is reporting the wrong capacity")
	}
	if capacityRemaining != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("new storage folder capacity remaining is reporting the wrong remaining capacity")
	}
	mfi, err = os.Stat(mfn)
	if err != nil {
		t.Fatal(err)
	}
	sfi, err = os.Stat(sfn)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*2 {
		t.Error("metadata file is the wrong size")
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("sector file is the wrong size")
	}

	// Verify that every single sector is readable and has the correct data.
	wg.Add(len(roots))
	misses = 0
	for i := 0; i < len(roots); i++ {
		go func(i int) {
			data, err := cmt.cm.ReadSector(roots[i])
			if err != nil || !bytes.Equal(data, datas[i]) {
				atomic.AddUint64(&misses, 1)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
	if misses != 0 {
		t.Errorf("Could not find all %v sectors: %v\n", len(roots), misses)
	}
}

// TestShrinkSingleStorageFolder verifies that it's possible to shirnk a single
// storage folder with no destination for the sectors.
func TestShrinkSingleStorageFolder(t *testing.T) {
	// TODO: Supporting in-place storage folder shrinking requires the
	// move-sector function to be able to recognize the storage folder that it
	// is currently using - right now it needs a storage folder lock to migrate
	// a sector in, and a storage folder lock to migrate a sector out, and
	// these locks are independent, so it cannot move a sector into the folder
	// that the sector is being moved out of.
	t.Skip("In-place shrinking not currently supported")
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder.
	storageFolderOne := filepath.Join(cmt.persistDir, "storageFolderOne")
	mfn := filepath.Join(storageFolderOne, metadataFile)
	sfn := filepath.Join(storageFolderOne, sectorFile)
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderOne, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderOne, modules.SectorSize*storageFolderGranularity*8)
	if err != nil {
		t.Fatal(err)
	}
	// Get the index of the storage folder.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("there should only be one storage folder")
	}
	sfIndex := sfs[0].Index

	// Create some sectors and add them to the storage folder.
	roots := make([]crypto.Hash, storageFolderGranularity*3)
	datas := make([][]byte, storageFolderGranularity*3)
	for i := 0; i < storageFolderGranularity*3; i++ {
		root, data := randSector()
		roots[i] = root
		datas[i] = data
	}
	// Add all of the sectors.
	var wg sync.WaitGroup
	wg.Add(len(roots))
	for i := 0; i < len(roots); i++ {
		go func(i int) {
			err := cmt.cm.AddSector(roots[i], datas[i])
			if err != nil {
				t.Error(err)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()

	// Decrease the size of the storage folder.
	err = cmt.cm.ResizeStorageFolder(sfIndex, modules.SectorSize*storageFolderGranularity*4, false)
	if err != nil {
		t.Fatal(err)
	}
	// Verify that the capacity and file sizes are correct.
	sfs = cmt.cm.StorageFolders()
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*4 {
		t.Error("new storage folder is reporting the wrong capacity")
	}
	if sfs[0].CapacityRemaining != modules.SectorSize*storageFolderGranularity*1 {
		t.Error("new storage folder capacity remaining is reporting the wrong remaining capacity")
	}
	mfi, err := os.Stat(mfn)
	if err != nil {
		t.Fatal(err)
	}
	sfi, err := os.Stat(sfn)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*4 {
		t.Error("metadata file is the wrong size")
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*4 {
		t.Error("sector file is the wrong size")
	}

	// Verify that every single sector is readable and has the correct data.
	wg.Add(len(roots))
	misses := uint64(0)
	for i := 0; i < len(roots); i++ {
		go func(i int) {
			data, err := cmt.cm.ReadSector(roots[i])
			if err != nil || !bytes.Equal(data, datas[i]) {
				atomic.AddUint64(&misses, 1)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
	if misses != 0 {
		t.Errorf("Could not find all %v sectors: %v\n", len(roots), misses)
	}

	// Restart the contract manager to see that the change is persistent.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the capacity and file sizes are correct.
	sfs = cmt.cm.StorageFolders()
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*4 {
		t.Error("new storage folder is reporting the wrong capacity")
	}
	if sfs[0].CapacityRemaining != modules.SectorSize*storageFolderGranularity*1 {
		t.Error("new storage folder capacity remaining is reporting the wrong remaining capacity")
	}
	mfi, err = os.Stat(mfn)
	if err != nil {
		t.Fatal(err)
	}
	sfi, err = os.Stat(sfn)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*4 {
		t.Error("metadata file is the wrong size")
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*4 {
		t.Error("sector file is the wrong size")
	}

	// Verify that every single sector is readable and has the correct data.
	wg.Add(len(roots))
	misses = 0
	for i := 0; i < len(roots); i++ {
		go func(i int) {
			data, err := cmt.cm.ReadSector(roots[i])
			if err != nil || !bytes.Equal(data, datas[i]) {
				atomic.AddUint64(&misses, 1)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
	if misses != 0 {
		t.Errorf("Could not find all %v sectors: %v\n", len(roots), misses)
	}
}
