package contractmanager

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"gitlab.com/NebulousLabs/Sia/modules"
)

// TestGrowStorageFolder checks that a storage folder can be successfully
// increased in size.
func TestGrowStorageFolder(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester("TestGrowStorageFolder")
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
	err = cmt.cm.AddStorageFolder(storageFolderOne, modules.SectorSize*storageFolderGranularity)
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
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity {
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
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity {
		t.Error("metadata file is the wrong size")
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity {
		t.Error("sector file is the wrong size")
	}

	// Increase the size of the storage folder.
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

// dependencyIncompleteGrow will start to have disk failures after too much
// data is written and also after 'triggered' ahs been set to true.
type dependencyIncompleteGrow struct {
	modules.ProductionDependencies
	triggered bool
	threshold int
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

// CreateFile will return a file that will return an error if a write will put
// the total throughput of the file over 1 MiB.
func (dig *dependencyIncompleteGrow) CreateFile(s string) (modules.File, error) {
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
	if l.throughput >= l.dig.threshold {
		return 0, errors.New("triggerLimitFile throughput limit reached earlier")
	}

	// If the limit has not been reached, pass the call through to the
	// underlying file.
	if l.throughput+len(b) <= l.dig.threshold {
		l.throughput += len(b)
		return l.File.WriteAt(b, offset)
	}

	// If the limit has been reached, write enough bytes to get to 8 MiB, then
	// return an error.
	remaining := l.dig.threshold - l.throughput
	l.throughput = l.dig.threshold
	written, err := l.File.WriteAt(b[:remaining], offset)
	if err != nil {
		return written, err
	}
	return written, errors.New("triggerLimitFile throughput limit reached before all input was written to disk")
}

// Truncate returns an error if the operation will put the total throughput of
// the file over 8 MiB.
func (l *triggerLimitFile) Truncate(offset int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.dig.mu.Lock()
	triggered := l.dig.triggered
	l.dig.mu.Unlock()
	if !triggered {
		return l.File.Truncate(offset)
	}

	// If the limit has already been reached, return an error.
	if l.throughput >= l.dig.threshold {
		return errors.New("triggerLimitFile throughput limit reached earlier")
	}

	// Get the file size, so we know what the throughput is.
	fi, err := l.Stat()
	if err != nil {
		return errors.New("triggerLimitFile unable to get FileInfo: " + err.Error())
	}

	// Run truncate with 0 throughput if size is larger than offset.
	if fi.Size() > offset {
		return l.File.Truncate(offset)
	}

	writeSize := int(offset - fi.Size())

	// If the limit has not been reached, pass the call through to the
	// underlying file.
	if l.throughput+writeSize <= l.dig.threshold {
		l.throughput += writeSize
		return l.File.Truncate(offset)
	}

	// If the limit has been reached, return an error.
	// return an error.
	return errors.New("triggerLimitFile throughput limit reached, no ability to allocate more")
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
	d.threshold = 1 << 20
	d.triggered = true
	d.mu.Unlock()

	// Increase the size of the storage folder, to large enough that it will
	// fail.
	err = cmt.cm.ResizeStorageFolder(sfIndex, modules.SectorSize*storageFolderGranularity*25, false)
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

// dependencyGrowNoFinalize will not add a confirmation to the WAL that a
// growStorageFolder operation has completed.
type dependencyGrowNoFinalize struct {
	modules.ProductionDependencies
}

// disrupt will prevent the growStorageFolder operation from committing a
// finalized growStorageFolder operation to the WAL.
func (*dependencyGrowNoFinalize) Disrupt(s string) bool {
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
	err = cmt.cm.ResizeStorageFolder(sfIndex, modules.SectorSize*storageFolderGranularity*25, false)
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
	modules.ProductionDependencies
	triggered bool
}

// disrupt will prevent the WAL file from being removed at shutdown.
func (dlw *dependencyLeaveWAL) Disrupt(s string) bool {
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
	err = cmt.cm.ResizeStorageFolder(sfIndex, modules.SectorSize*storageFolderGranularity*25, false)
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
