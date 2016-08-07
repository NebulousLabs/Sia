package contractmanager

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	if err != nil {
		t.Fatal(err)
	}
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

// TestAddStorageFolderConcurrent adds multiple storage folders concurrently to
// the contract manager.
func TestAddStorageFolderConcurrent(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
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
		err := cmt.cm.AddStorageFolder(storageFolderOne, modules.SectorSize*32*8)
		if err != nil {
			t.Fatal(err)
		}
		wg.Done()
	}()
	go func() {
		err := cmt.cm.AddStorageFolder(storageFolderTwo, modules.SectorSize*32*8)
		if err != nil {
			t.Fatal(err)
		}
		wg.Done()
	}()
	go func() {
		err = cmt.cm.AddStorageFolder(storageFolderThree, modules.SectorSize*32*8)
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
	sfOneSize := modules.SectorSize * 32 * 8
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
	if sfs[0].ProgressDenominator != sfOneSize+16*32*8 {
		// The 16*32*8 comes from the fact that there are 8*32 sectors storage
		// folder one, and that 16 additional bytes are needed per sector to
		// store metadata.
		t.Error("storage folder is not showing that an action is in progress, though one is", sfs[0].ProgressDenominator, sfOneSize)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		err := cmt.cm.AddStorageFolder(storageFolderTwo, modules.SectorSize*32*8)
		if err != nil {
			t.Fatal(err)
		}
		wg.Done()
	}()
	go func() {
		err = cmt.cm.AddStorageFolder(storageFolderThree, modules.SectorSize*32*8)
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
