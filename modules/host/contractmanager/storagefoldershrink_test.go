package contractmanager

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
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
	err = cmt.cm.ResizeStorageFolder(sfIndex, modules.SectorSize*storageFolderGranularity*2)
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
	cmt, err := newContractManagerTester("TestShrinkStorageFolderWithSectors")
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
		root, data, err := randSector()
		if err != nil {
			t.Fatal(err)
		}
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
	err = cmt.cm.ResizeStorageFolder(sfIndex, modules.SectorSize*storageFolderGranularity*2)
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

/*
// dependencyIncompleteGrow will start to have disk failures after too much
// data is written and also after 'triggered' ahs been set to true.
type dependencyIncompleteGrow struct {
	productionDependencies
	triggered bool
	mu        sync.Mutex
}

// triggerLimitFile will return an error if a call to Write is made that will
// put the total throughput of the file over 1 MiB. Counting only begins once
// triggered.
type triggerLimitFile struct {
	dig *dependencyIncompleteGrow

	throughput int
	mu         sync.Mutex
	*os.File
	sync.Mutex
}

// createFile will return a file that will return an error if a write will put
// the total throughput of the file over 1 MiB.
func (dig *dependencyIncompleteGrow) createFile(s string) (file, error) {
	osFile, err := os.Create(s)
	if err != nil {
		return nil, err
	}

	tlf := &triggerLimitFile{
		dig:  dig,
		File: osFile,
	}
	return tlf, nil
}

// Write returns an error if the operation will put the total throughput of the
// file over 8 MiB. The write will write all the way to 8 MiB before returning
// the error.
func (l *triggerLimitFile) WriteAt(b []byte, offset int64) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.dig.mu.Lock()
	triggered := l.dig.triggered
	l.dig.mu.Unlock()
	if !triggered {
		return l.File.WriteAt(b, offset)
	}

	// If the limit has already been reached, return an error.
	if l.throughput >= 1<<20 {
		return 0, errors.New("triggerLimitFile throughput limit reached earlier")
	}

	// If the limit has not been reached, pass the call through to the
	// underlying file.
	if l.throughput+len(b) <= 1<<20 {
		l.throughput += len(b)
		return l.File.WriteAt(b, offset)
	}

	// If the limit has been reached, write enough bytes to get to 8 MiB, then
	// return an error.
	remaining := 1<<20 - l.throughput
	l.throughput = 1 << 20
	written, err := l.File.WriteAt(b[:remaining], offset)
	if err != nil {
		return written, err
	}
	return written, errors.New("triggerLimitFile throughput limit reached before all input was written to disk")
}

// TestGrowStorageFolderIncopmleteWrite checks that growStorageFolder operates
// as intended when the writing to increase the filesize does not complete all
// the way.
func TestGrowStorageFolderIncompleteWrite(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	d := new(dependencyIncompleteGrow)
	cmt, err := newMockedContractManagerTester(d, "TestGrowStorageFolderIncompleteWrite")
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
	err = cmt.cm.AddStorageFolder(storageFolderOne, modules.SectorSize*storageFolderGranularity*3)
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
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*3 {
		t.Error("new storage folder is reporting the wrong capacity")
	}

	// Trigger the dependencies, so that writes begin failing.
	d.mu.Lock()
	d.triggered = true
	d.mu.Unlock()

	// Increase the size of the storage folder, to large enough that it will
	// fail.
	err = cmt.cm.ResizeStorageFolder(sfIndex, modules.SectorSize*storageFolderGranularity*25)
	if err == nil {
		t.Fatal("expecting error upon resize")
	}

	// Verify that the storage folder has the correct capacity.
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*3 {
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
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*3 {
		t.Error("metadata file is the wrong size:", mfi.Size(), sectorMetadataDiskSize*storageFolderGranularity*3)
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*3 {
		t.Error("sector file is the wrong size:", sfi.Size(), modules.SectorSize*storageFolderGranularity*3)
	}

	// Restart the contract manager.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the storage folder has the correct capacity.
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*3 {
		t.Error("new storage folder is reporting the wrong capacity")
	}
	// Verify that the on-disk files are the right size.
	mfi, err = os.Stat(mfn)
	if err != nil {
		t.Fatal(err)
	}
	sfi, err = os.Stat(sfn)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*3 {
		t.Error("metadata file is the wrong size:", mfi.Size(), sectorMetadataDiskSize*storageFolderGranularity*3)
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*3 {
		t.Error("sector file is the wrong size:", sfi.Size(), modules.SectorSize*storageFolderGranularity*3)
	}
}

// dependencyGrowNoFinalize will start to have disk failures after too much
// data is written and also after 'triggered' ahs been set to true.
type dependencyGrowNoFinalize struct {
	productionDependencies
}

// disrupt will prevent the growStorageFolder operation from committing a
// finalized growStorageFolder operation to the WAL.
func (dependencyGrowNoFinalize) disrupt(s string) bool {
	if s == "incompleteGrowStorageFolder" {
		return true
	}
	if s == "cleanWALFile" {
		return true
	}
	return false
}

// TestGrowStorageFolderShutdownAfterWrite simulates an unclean shutdown that
// occurs after the storage folder write has completed, but before it has
// established through the WAL that the write has completed. The result should
// be that the storage folder grow is not accepted after restart.
func TestGrowStorageFolderShutdownAfterWrite(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	d := new(dependencyGrowNoFinalize)
	cmt, err := newMockedContractManagerTester(d, "TestGrowStorageFolderShutdownAfterWrite")
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
	err = cmt.cm.AddStorageFolder(storageFolderOne, modules.SectorSize*storageFolderGranularity*3)
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
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*3 {
		t.Error("new storage folder is reporting the wrong capacity")
	}

	// Increase the size of the storage folder, to large enough that it will
	// fail.
	err = cmt.cm.ResizeStorageFolder(sfIndex, modules.SectorSize*storageFolderGranularity*25)
	if err != nil {
		t.Fatal(err)
	}

	// Restart the contract manager.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the storage folder has the correct capacity.
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*3 {
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
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*3 {
		t.Error("metadata file is the wrong size:", mfi.Size(), sectorMetadataDiskSize*storageFolderGranularity*3)
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*3 {
		t.Error("sector file is the wrong size:", sfi.Size(), modules.SectorSize*storageFolderGranularity*3)
	}
}

// dependencyLeaveWAL will leave the WAL on disk during shutdown.
type dependencyLeaveWAL struct {
	mu sync.Mutex
	productionDependencies
	triggered bool
}

// disrupt will prevent the WAL file from being removed at shutdown.
func (dlw *dependencyLeaveWAL) disrupt(s string) bool {
	if s == "cleanWALFile" {
		return true
	}

	dlw.mu.Lock()
	triggered := dlw.triggered
	dlw.mu.Unlock()
	if s == "walRename" && triggered {
		return true
	}

	return false
}

// TestGrowStorageFolderWAL completes a storage folder growing, but leaves the
// WAL behind so that a commit is necessary to finalize things.
func TestGrowStorageFolderWAL(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	d := new(dependencyLeaveWAL)
	cmt, err := newMockedContractManagerTester(d, "TestGrowStorageFolderWAL")
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
	err = cmt.cm.AddStorageFolder(storageFolderOne, modules.SectorSize*storageFolderGranularity*3)
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
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*3 {
		t.Error("new storage folder is reporting the wrong capacity")
	}

	// Increase the size of the storage folder, to large enough that it will
	// fail.
	err = cmt.cm.ResizeStorageFolder(sfIndex, modules.SectorSize*storageFolderGranularity*25)
	if err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	d.triggered = true
	d.mu.Unlock()

	// Restart the contract manager.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the storage folder has the correct capacity.
	sfs = cmt.cm.StorageFolders()
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*25 {
		t.Error("new storage folder is reporting the wrong capacity", sfs[0].Capacity/modules.SectorSize, storageFolderGranularity*25)
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
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*25 {
		t.Error("metadata file is the wrong size:", mfi.Size(), sectorMetadataDiskSize*storageFolderGranularity*25)
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*25 {
		t.Error("sector file is the wrong size:", sfi.Size(), modules.SectorSize*storageFolderGranularity*25)
	}
}
*/
