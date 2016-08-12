package contractmanager

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
)

// TestAddStorageFolder tries to add a storage folder to the contract manager,
// blocking until the add has completed.
func TestAddStorageFolder(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester("TestAddStorageFolder")
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
	err = cmt.cm.AddStorageFolder(storageFolderDir, modules.SectorSize*64*2)
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
	if sfs[0].Capacity != modules.SectorSize*64*2 {
		t.Error("storage folder reported with wrong sector size")
	}
}

// dependencyLargeFolder is a mocked dependency that will return files which
// can only handle 1 MiB of data being written to them.
type dependencyLargeFolder struct {
	productionDependencies
}

// limitFile will return an error if a call to Write is made that will put the
// total throughput of the file over 1 MiB.
type limitFile struct {
	throughput int
	mu         sync.Mutex
	*os.File
}

// createFile will return a file that will return an error if a write will put
// the total throughput of the file over 1 MiB.
func (dependencyLargeFolder) createFile(s string) (file, error) {
	osFile, err := os.Create(s)
	if err != nil {
		return osFile, err
	}

	lf := &limitFile{
		File: osFile,
	}
	return lf, nil
}

// Write returns an error if the operation will put the total throughput of the
// file over 8 MiB. The write will write all the way to 8 MiB before returning
// the error.
func (l *limitFile) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	// If the limit has already been reached, return an error.
	if l.throughput >= 1<<20 {
		return 0, errors.New("limitFile throughput limit reached earlier")
	}

	// If the limit has not been reached, pass the call through to the
	// underlying file.
	if l.throughput+len(b) <= 1<<20 {
		l.throughput += len(b)
		return l.File.Write(b)
	}

	// If the limit has been reached, write enough bytes to get to 8 MiB, then
	// return an error.
	remaining := 1<<20 - l.throughput
	l.throughput = 1 << 20
	written, err := l.File.Write(b[:remaining])
	if err != nil {
		return written, err
	}
	return written, errors.New("limitFile throughput limit reached before all input was written to disk")
}

// TestAddLargeStorageFolder tries to add a storage folder that is too large to
// fit on disk. This is represented by mocking a file that returns an error
// after more than 8 MiB have been written.
func TestAddLargeStorageFolder(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	var d dependencyLargeFolder
	cmt, err := newMockedContractManagerTester(d, "TestAddLargeStorageFolder")
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
	err = cmt.cm.AddStorageFolder(storageFolderDir, modules.SectorSize*64*16) // Total size must exceed the limit of the limitFile.
	// Should be a storage folder error, but with all the context adding, I'm
	// not sure how to check the error type.
	if err == nil {
		t.Fatal(err)
	}

	// Check that the storage folder has been added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 0 {
		t.Fatal("Storage folder add should have failed.")
	}
	// Check that the storage folder is empty - because the operation failed,
	// any files that got created should have been removed.
	files, err := ioutil.ReadDir(storageFolderDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Error("there should not be any files in the storage folder because the AddStorageFolder operation failed.")
	}
}

// TestAddStorageFolderConcurrent adds multiple storage folders concurrently to
// the contract manager.
func TestAddStorageFolderConcurrent(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester("TestAddStorageFolderConcurrent")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder to the contract manager tester.
	storageFolderOne := filepath.Join(cmt.persistDir, "storageFolderOne")
	storageFolderTwo := filepath.Join(cmt.persistDir, "storageFolderTwo")
	storageFolderThree := filepath.Join(cmt.persistDir, "storageFolderThree")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderOne, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll(storageFolderTwo, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll(storageFolderThree, 0700)
	if err != nil {
		t.Fatal(err)
	}

	// Launch three calls to add simultaneously and wait for all three to
	// finish.
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		err := cmt.cm.AddStorageFolder(storageFolderOne, modules.SectorSize*64*8)
		if err != nil {
			t.Fatal(err)
		}
		wg.Done()
	}()
	go func() {
		err := cmt.cm.AddStorageFolder(storageFolderTwo, modules.SectorSize*64*8)
		if err != nil {
			t.Fatal(err)
		}
		wg.Done()
	}()
	go func() {
		err = cmt.cm.AddStorageFolder(storageFolderThree, modules.SectorSize*64*8)
		if err != nil {
			t.Fatal(err)
		}
		wg.Done()
	}()
	wg.Wait()

	// Check that the storage folder has been added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 3 {
		t.Fatal("There should be one storage folder reported")
	}
}

// dependencyBlockSFOne is a mocked dependency for os.Create that will return a
// file for storage folder one only which will block on a call to file.Write
// until a signal has been given that the block can be released.
type dependencyBlockSFOne struct {
	blockLifted chan struct{}
	writeCalled chan struct{}
	productionDependencies
}

// blockedFile is the file that gets returned by dependencyBlockSFOne to
// storageFolderOne.
type blockedFile struct {
	blockLifted chan struct{}
	writeCalled chan struct{}
	*os.File
}

// Write will block until a signal is given that the block may be lifted. Write
// will signal when it has been called for the first time, so that the tester
// knows the function has reached a blocking point.
func (bf *blockedFile) Write(b []byte) (int, error) {
	close(bf.writeCalled)
	<-bf.blockLifted
	return bf.File.Write(b)
}

// createFile will return a normal file to all callers except for
// storageFolderOne, which will have calls to file.Write blocked until a signal
// is given that the blocks may be released.
func (d *dependencyBlockSFOne) createFile(s string) (file, error) {
	// If storageFolderOne, return a file that will not write until the signal
	// is sent that writing is okay.
	if strings.Contains(s, "storageFolderOne") {
		file, err := os.Create(s)
		if err != nil {
			return nil, err
		}
		bf := &blockedFile{
			blockLifted: d.blockLifted,
			writeCalled: d.writeCalled,
			File:        file,
		}
		return bf, nil
	}

	// If not storageFolderOne, return a normal file.
	return os.Create(s)
}

// TestAddStorageFolderBlocking adds multiple storage folders concurrently to
// the contract manager, blocking on the first one to make sure that the others
// are still allowed to complete.
func TestAddStorageFolderBlocking(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// Create the mocked dependencies that will block for the first storage
	// folder.
	d := &dependencyBlockSFOne{
		blockLifted: make(chan struct{}),
		writeCalled: make(chan struct{}),
	}

	// Create a contract manager tester with the mocked dependencies.
	cmt, err := newMockedContractManagerTester(d, "TestAddStorageFolderBlocking")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder to the contract manager tester.
	storageFolderOne := filepath.Join(cmt.persistDir, "storageFolderOne")
	storageFolderTwo := filepath.Join(cmt.persistDir, "storageFolderTwo")
	storageFolderThree := filepath.Join(cmt.persistDir, "storageFolderThree")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderOne, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll(storageFolderTwo, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll(storageFolderThree, 0700)
	if err != nil {
		t.Fatal(err)
	}

	// Spin off the first goroutine, and then wait until write has been called
	// on the underlying file.
	sfOneSize := modules.SectorSize * 64 * 8
	go func() {
		err := cmt.cm.AddStorageFolder(storageFolderOne, sfOneSize)
		if err != nil {
			t.Fatal(err)
		}
	}()
	<-d.writeCalled

	// Check the status of the storage folder. At this point, the folder should
	// be returned as an unfinished storage folder addition, with progress
	// indicating that the storage folder is at 0 bytes progressed out of
	// sfOneSize.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("there should be one storage folder reported")
	}
	if sfs[0].ProgressNumerator != 0 {
		t.Error("storage folder is showing progress despite being blocked")
	}
	if sfs[0].ProgressDenominator != sfOneSize+16*64*8 {
		// The 16*64*8 comes from the fact that there are 8*64 sectors storage
		// folder one, and that 16 additional bytes are needed per sector to
		// store metadata.
		t.Error("storage folder is not showing that an action is in progress, though one is", sfs[0].ProgressDenominator, sfOneSize)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		err := cmt.cm.AddStorageFolder(storageFolderTwo, modules.SectorSize*64*8)
		if err != nil {
			t.Fatal(err)
		}
		wg.Done()
	}()
	go func() {
		err = cmt.cm.AddStorageFolder(storageFolderThree, modules.SectorSize*64*8)
		if err != nil {
			t.Fatal(err)
		}
		wg.Done()
	}()
	wg.Wait()
	close(d.blockLifted)
	cmt.cm.tg.Flush()

	// Check that the storage folder has been added.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 3 {
		t.Fatal("There should be one storage folder reported")
	}
	// All actions should have completed, so all storage folders should be
	// reporting '0' in the progress denominator.
	for _, sf := range sfs {
		if sf.ProgressDenominator != 0 {
			t.Error("ProgressDenominator is indicating that actions still remain")
		}
	}
}

// TestAddStorageFolderConsecutive adds multiple storage folders consecutively
// to the contract manager, blocking on the first one to make sure that the
// others are still allowed to complete.
func TestAddStorageFolderConsecutive(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// Create a contract manager tester with the mocked dependencies.
	cmt, err := newContractManagerTester("TestAddStorageFolderConsecutive")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder to the contract manager tester.
	storageFolderOne := filepath.Join(cmt.persistDir, "storageFolderOne")
	storageFolderTwo := filepath.Join(cmt.persistDir, "storageFolderTwo")
	storageFolderThree := filepath.Join(cmt.persistDir, "storageFolderThree")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderOne, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll(storageFolderTwo, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll(storageFolderThree, 0700)
	if err != nil {
		t.Fatal(err)
	}

	// Spin off the first goroutine, and then wait until write has been called
	// on the underlying file.
	sfSize := modules.SectorSize * 64 * 8
	err = cmt.cm.AddStorageFolder(storageFolderOne, sfSize)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderTwo, sfSize)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderThree, sfSize)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the storage folder has been added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 3 {
		t.Fatal("There should be one storage folder reported")
	}
	// All actions should have completed, so all storage folders should be
	// reporting '0' in the progress denominator.
	for _, sf := range sfs {
		if sf.ProgressDenominator != 0 {
			t.Error("ProgressDenominator is indicating that actions still remain")
		}
	}
}

// TestAddStorageFolderDoubleAdd concurrently adds two storage
// folders with the same path to the contract manager.
func TestAddStorageFolderDoubleAdd(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// Create a contract manager tester with the mocked dependencies.
	cmt, err := newContractManagerTester("TestAddStorageFolderDoubleAdd")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder to the contract manager tester.
	storageFolderOne := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderOne, 0700)
	if err != nil {
		t.Fatal(err)
	}

	// Call AddStorageFolder in three separate goroutines, where the same path
	// is used in each. The errors are not checked because one of the storage
	// folders will succeed, but it's uncertain which one.
	sfSize := modules.SectorSize * 64 * 8
	err = cmt.cm.AddStorageFolder(storageFolderOne, sfSize)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderOne, sfSize*2)
	if err != errRepeatFolder {
		t.Fatal(err)
	}

	// Check that the storage folder has been added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder reported")
	}
	// All actions should have completed, so all storage folders should be
	// reporting '0' in the progress denominator
	for _, sf := range sfs {
		if sf.ProgressDenominator != 0 {
			t.Error("ProgressDenominator is indicating that actions still remain")
		}
	}
}

// dependencyNoSyncLoop is a mocked dependency that will disable the sync loop.
type dependencyNoSyncLoop struct {
	productionDependencies
}

// disrupt will disrupt the threadedSyncLoop, causing the loop to terminate as
// soon as it is created.
func (dependencyNoSyncLoop) disrupt(s string) bool {
	if s == "threadedSyncLoopStart" || s == "cleanWALFile" {
		// Disrupt threadedSyncLoop. The sync loop will exit immediately
		// instead of executing commits. Also disrupt the process that removes
		// the WAL file following clean shutdown.
		return true
	}
	return false
}

// TestAddStorageFolderDoubleAddNoCommit hijacks the sync loop in the contract
// manager such that the sync loop will not run automatically. Then, without
// doing an actual commit, the test will indicate to open functions that a
// commit has completed, allowing the next storage folder operation to happen.
// Because the changes were finalized but not committed, extra code coverage
// should be achieved, though the result of the storage folder being rejected
// should be the same.
func TestAddStorageFolderDoubleAddNoCommit(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	var d dependencyNoSyncLoop
	cmt, err := newMockedContractManagerTester(d, "TestAddStorageFolderDoubleAddNoCommit")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// The sync loop will never run, which means naively AddStorageFolder will
	// never return. To get AddStorageFolder to return before the commit
	// completes, spin up an alternate sync loop which only performs the
	// signaling responsibilities of the commit function.
	closeFakeSyncChan := make(chan struct{})
	go func() {
		for {
			select {
			case <-closeFakeSyncChan:
				return
			case <-time.After(time.Millisecond * 250):
				// Signal that the commit operation has completed, even though
				// it has not.
				cmt.cm.wal.mu.Lock()
				close(cmt.cm.wal.syncChan)
				cmt.cm.wal.syncChan = make(chan struct{})
				cmt.cm.wal.mu.Unlock()
			}
		}
	}()
	defer close(closeFakeSyncChan)

	// Add a storage folder to the contract manager tester.
	storageFolderOne := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderOne, 0700)
	if err != nil {
		t.Fatal(err)
	}

	// Call AddStorageFolder in three separate goroutines, where the same path
	// is used in each. The errors are not checked because one of the storage
	// folders will succeed, but it's uncertain which one.
	sfSize := modules.SectorSize * 64 * 8
	err = cmt.cm.AddStorageFolder(storageFolderOne, sfSize)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderOne, sfSize*2)
	if err != errRepeatFolder {
		t.Fatal(err)
	}

	// Check that the storage folder has been added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder reported", len(sfs))
	}
	// All actions should have completed, so all storage folders should be
	// reporting '0' in the progress denominator
	for _, sf := range sfs {
		if sf.ProgressDenominator != 0 {
			t.Error("ProgressDenominator is indicating that actions still remain")
		}
	}
}

// TestAddStorageFolderFailedCommit utilizes the sync-loop hijacking in
// TestAddStorageFolderDoubleAddNoCommit to create a commit scheme that syncs
// the WAL, but does not actually commit the actions. This simulates a disk
// failure or power failure, resulting in a partial-completion of the storage
// folder addition.
func TestAddStorageFolderFailedCommit(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	var d dependencyNoSyncLoop
	cmt, err := newMockedContractManagerTester(d, "TestAddStorageFolderFailedCommit")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// The sync loop will never run, which means naively AddStorageFolder will
	// never return. To get AddStorageFolder to return before the commit
	// completes, spin up an alternate sync loop which only performs the
	// signaling responsibilities of the commit function.
	//
	// This new sync-loop will also write the WAL file to disk and sync the
	// write, but will not properly call the commit() action.
	closeFakeSyncChan := make(chan struct{})
	go func() {
		for {
			select {
			case <-closeFakeSyncChan:
				return
			case <-time.After(time.Millisecond * 250):
				// Signal that the commit operation has completed, even though
				// it has not.
				cmt.cm.wal.mu.Lock()
				close(cmt.cm.wal.syncChan)
				cmt.cm.wal.syncChan = make(chan struct{})
				cmt.cm.wal.mu.Unlock()
			}
		}
	}()

	// Add a storage folder to the contract manager tester.
	storageFolderOne := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderOne, 0700)
	if err != nil {
		t.Fatal(err)
	}

	// Call AddStorageFolder, knowing that the changes will not be properly
	// committed.
	sfSize := modules.SectorSize * 64 * 8
	err = cmt.cm.AddStorageFolder(storageFolderOne, sfSize)
	if err != nil {
		t.Fatal(err)
	}

	// Perform the sync cycle with the WAL (hijacked sync loop will not), but
	// skip the part where the changes are actually committed.
	err = cmt.cm.wal.fileWALTmp.Sync()
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.wal.fileWALTmp.Close()
	if err != nil {
		t.Fatal(err)
	}
	walTmpName := filepath.Join(cmt.cm.persistDir, walFileTmp)
	walFileName := filepath.Join(cmt.cm.persistDir, walFile)
	err = os.Rename(walTmpName, walFileName)
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm.wal.fileWALTmp, err = os.Create(walTmpName)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the storage folder has been added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder reported")
	}
	// All actions should have completed, so all storage folders should be
	// reporting '0' in the progress denominator
	if sfs[0].ProgressDenominator != 0 {
		t.Error("ProgressDenominator is indicating that actions still remain")
	}

	// Close the contract manager and replace it with a new contract manager.
	// The new contract manager should have normal dependencies.
	close(closeFakeSyncChan)
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
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder reported", len(sfs))
	}
}

// dependencyNoSyncBadAdd is a mocked dependency that will disable the sync
// loop and prevent AddStorageFolder from successfully completing.
type dependencyNoSyncBadAdd struct {
	productionDependencies
}

// disrupt will disrupt the threadedSyncLoop, causing the loop to terminate as
// soon as it is created.
func (dependencyNoSyncBadAdd) disrupt(s string) bool {
	if s == "threadedSyncLoopStart" || s == "incompleteAddStorageFolder" || s == "cleanWALFile" {
		// Disrupt threadedSyncLoop. The sync loop will exit immediately
		// instead of executing commits.
		return true
	}
	return false
}

// TestAddStorageFolderUnfinishedCreate hijacks both the sync loop and the
// AddStorageFolder code to create a situation where the added storage folder
// is started but not seen through to conclusion, and no commit is run.
func TestAddStorageFolderUnfinishedCreate(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	var d dependencyNoSyncBadAdd
	cmt, err := newMockedContractManagerTester(d, "TestAddStorageFolderUnfinishedCreate")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// The sync loop will never run, which means naively AddStorageFolder will
	// never return. To get AddStorageFolder to return before the commit
	// completes, spin up an alternate sync loop which only performs the
	// signaling responsibilities of the commit function.
	//
	// This new sync-loop will also write the WAL file to disk and sync the
	// write, but will not properly call the commit() action.
	closeFakeSyncChan := make(chan struct{})
	go func() {
		for {
			select {
			case <-closeFakeSyncChan:
				return
			case <-time.After(time.Millisecond * 250):
				// Signal that the commit operation has completed, even though
				// it has not.
				cmt.cm.wal.mu.Lock()
				close(cmt.cm.wal.syncChan)
				cmt.cm.wal.syncChan = make(chan struct{})
				cmt.cm.wal.mu.Unlock()
			}
		}
	}()

	// Add a storage folder to the contract manager tester.
	storageFolderOne := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderOne, 0700)
	if err != nil {
		t.Fatal(err)
	}
	// Call AddStorageFolder, knowing that the changes will not be properly
	// committed, and that the call itself will not actually complete.
	sfSize := modules.SectorSize * 64 * 8
	err = cmt.cm.AddStorageFolder(storageFolderOne, sfSize)
	if err != nil {
		t.Fatal(err)
	}

	// Perform the sync cycle with the WAL (hijacked sync loop will not), but
	// skip the part where the changes are actually committed.
	err = cmt.cm.wal.fileWALTmp.Sync()
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.wal.fileWALTmp.Close()
	if err != nil {
		t.Fatal(err)
	}
	walTmpName := filepath.Join(cmt.cm.persistDir, walFileTmp)
	walFileName := filepath.Join(cmt.cm.persistDir, walFile)
	err = os.Rename(walTmpName, walFileName)
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm.wal.fileWALTmp, err = os.Create(walTmpName)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the storage folder has been added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder reported")
	}

	// Close the contract manager and replace it with a new contract manager.
	// The new contract manager should have normal dependencies.
	close(closeFakeSyncChan)
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
	// Check that the storage folder was properly removed - incomplete storage
	// folder adds should be removed upon startup.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 0 {
		t.Error("Storage folder add should have failed.")
	}
	// Check that the storage folder is empty - because the operation failed,
	// any files that got created should have been removed.
	files, err := ioutil.ReadDir(storageFolderOne)
	if err != nil {
		t.Error(err)
	}
	if len(files) != 0 {
		t.Error("there should not be any files in the storage folder because the AddStorageFolder operation failed.", len(files))
	}
}

// TestAddStorageFolderDoubleAddConcurrent concurrently adds two storage
// folders with the same path to the contract manager.
func TestAddStorageFolderDoubleAddConcurrent(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// Create a contract manager tester with the mocked dependencies.
	cmt, err := newContractManagerTester("TestAddStorageFolderDoubleAddConcurrent")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder to the contract manager tester.
	storageFolderOne := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderOne, 0700)
	if err != nil {
		t.Fatal(err)
	}

	// Call AddStorageFolder in three separate goroutines, where the same path
	// is used in each. The errors are not checked because one of the storage
	// folders will succeed, but it's uncertain which one.
	var wg sync.WaitGroup
	sfSize := modules.SectorSize * 64 * 8
	wg.Add(3)
	go func() {
		_ = cmt.cm.AddStorageFolder(storageFolderOne, sfSize)
		wg.Done()
	}()
	go func() {
		_ = cmt.cm.AddStorageFolder(storageFolderOne, sfSize*2)
		wg.Done()
	}()
	go func() {
		_ = cmt.cm.AddStorageFolder(storageFolderOne, sfSize*3)
		wg.Done()
	}()
	wg.Wait()

	// Check that the storage folder has been added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder reported")
	}
	// All actions should have completed, so all storage folders should be
	// reporting '0' in the progress denominator.
	for _, sf := range sfs {
		if sf.ProgressDenominator != 0 {
			t.Error("ProgressDenominator is indicating that actions still remain")
		}
	}
}

// TestAddStorageFolderReload adds a storage folder to the contract manager,
// and then reloads the contract manager to see if the storage folder is still
// there.
func TestAddStorageFolderReload(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// Create a contract manager tester with the mocked dependencies.
	cmt, err := newContractManagerTester("TestAddStorageFolderReload")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder to the contract manager tester.
	storageFolderOne := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderOne, 0700)
	if err != nil {
		t.Fatal(err)
	}
	sfSize := modules.SectorSize * 64 * 24
	err = cmt.cm.AddStorageFolder(storageFolderOne, sfSize)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the storage folder has been added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder reported")
	}
	// Check that the size of the storage folder is correct.
	if sfs[0].Capacity != sfSize {
		t.Error("capacity reported by storage folder is not the capacity alloacted")
	}
	if sfs[0].CapacityRemaining != sfSize {
		t.Error("capacity remaining reported by storage folder is not the capacity alloacted")
	}
	// All actions should have completed, so all storage folders should be
	// reporting '0' in the progress denominator.
	for _, sf := range sfs {
		if sf.ProgressDenominator != 0 {
			t.Error("ProgressDenominator is indicating that actions still remain")
		}
	}

	// Close the contract manager and open a new one using the same
	// persistence.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the storage folder has been added.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder reported", len(sfs))
	}
	// Check that the size of the storage folder is correct.
	if sfs[0].Capacity != sfSize {
		t.Error("capacity reported by storage folder is not the capacity alloacted")
	}
	if sfs[0].CapacityRemaining != sfSize {
		t.Error("capacity remaining reported by storage folder is not the capacity alloacted")
	}
	// Check that the storage folder as represented on disk has the correct
	// size.
	sectorLookupTableSize := int64(64 * 24 * 16)
	expectedSize := int64(sfSize) + sectorLookupTableSize
	fi, err := os.Stat(filepath.Join(storageFolderOne, sectorFile))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != expectedSize {
		t.Error("sector file had unexpected size", fi.Size(), expectedSize)
	}
}
