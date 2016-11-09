package contractmanager

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

// contractManagerTester holds a contract manager along with some other fields
// useful for testing, and has methods implemented on it that can assist
// testing.
type contractManagerTester struct {
	cm *ContractManager

	persistDir string
}

// panicClose will attempt to call Close on the contract manager tester. If
// there is an error, the function will panic. A convenient function for making
// sure that the cleanup code is always running correctly, without needing to
// write a lot of boiler code.
func (cmt *contractManagerTester) panicClose() {
	err := cmt.Close()
	if err != nil {
		panic(err)
	}
}

// Close will perform clean shutdown on the contract manager tester.
func (cmt *contractManagerTester) Close() error {
	if cmt.cm == nil {
		return errors.New("nil contract manager")
	}
	return cmt.cm.Close()
}

// newContractManagerTester returns a ready-to-rock contract manager tester.
func newContractManagerTester(name string) (*contractManagerTester, error) {
	if testing.Short() {
		panic("use of newContractManagerTester during short testing")
	}

	testdir := build.TempDir(modules.ContractManagerDir, name)
	cm, err := New(filepath.Join(testdir, modules.ContractManagerDir))
	if err != nil {
		return nil, err
	}
	cmt := &contractManagerTester{
		cm:         cm,
		persistDir: testdir,
	}
	return cmt, nil
}

// newMockedContractManagerTester returns a contract manager tester that uses
// the input dependencies instead of the production ones.
func newMockedContractManagerTester(d dependencies, name string) (*contractManagerTester, error) {
	if testing.Short() {
		panic("use of newContractManagerTester during short testing")
	}

	testdir := build.TempDir(modules.ContractManagerDir, name)
	cm, err := newContractManager(d, filepath.Join(testdir, modules.ContractManagerDir))
	if err != nil {
		return nil, err
	}
	cmt := &contractManagerTester{
		cm:         cm,
		persistDir: testdir,
	}
	return cmt, nil
}

// TestNewContractManager does basic startup and shutdown of a contract
// manager, checking for egregious errors.
func TestNewContractManager(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create a contract manager.
	parentDir := build.TempDir(modules.ContractManagerDir, "TestNewContractManager")
	cmDir := filepath.Join(parentDir, modules.ContractManagerDir)
	cm, err := New(cmDir)
	if err != nil {
		t.Fatal(err)
	}
	// Close the contract manager.
	err = cm.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Create a new contract manager using the same directory.
	cm, err = New(cmDir)
	if err != nil {
		t.Fatal(err)
	}
	// Close it again.
	err = cm.Close()
	if err != nil {
		t.Fatal(err)
	}
}

// TestParallelFileAccess using a single file handle + ReadAt and WriteAt to
// write to multiple locations on a file in parallel, verifying that it's a
// safe thing to do.
func TestParallelFileAccess(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create the file that will be used in parallel.
	testdir := build.TempDir(modules.ContractManagerDir, "TestParallelFileAccess")
	err := os.Mkdir(testdir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(testdir, "parallelFile"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Create the data that will be writted to the file, such that it can be
	// verified later.
	writesPerThread := 200
	numThreads := 500
	dataSize := 163 // Intentionally overlaps sector boundaries.
	datas := make([][]byte, numThreads*writesPerThread)
	for i := 0; i < numThreads*writesPerThread; i++ {
		datas[i] = make([]byte, dataSize)
		crypto.Read(datas[i])
	}

	// Spin up threads to make concurrent writes to the file in different
	// locations. Have some reads + writes that are trying to overlap.
	threadingModifier := 71
	var wg1 sync.WaitGroup
	var wg2 sync.WaitGroup
	for i := 0; i < numThreads; i++ {
		if i%threadingModifier == 0 {
			wg1.Add(1)
		} else {
			wg2.Add(1)
		}
		go func(i int) {
			if i%threadingModifier == 0 {
				defer wg1.Done()
			} else {
				defer wg2.Done()
			}

			for j := 0; j < writesPerThread; j++ {
				_, err := f.WriteAt(datas[i*j], int64(i*dataSize*j))
				if err != nil {
					t.Error(err)
				}
			}
		}(i)
	}
	// Wait for the smaller set of first writes to complete.
	wg1.Wait()

	// Verify the results for the smaller set of writes.
	for i := 0; i < numThreads; i++ {
		if i%threadingModifier != 0 {
			continue
		}
		wg1.Add(1)
		go func(i int) {
			defer wg1.Done()
			for j := 0; j < writesPerThread; j++ {
				data := make([]byte, dataSize)
				_, err := f.ReadAt(data, int64(i*dataSize))
				if err != nil {
					t.Error(err)
				}
				if !bytes.Equal(data, datas[i]) {
					t.Error("data mismatch for value", i)
				}
			}
		}(i)
	}
	wg1.Wait()
	wg2.Wait()

	// Verify the results for all of the writes.
	for i := 0; i < numThreads; i++ {
		wg1.Add(1)
		go func(i int) {
			defer wg1.Done()
			for j := 0; j < writesPerThread; j++ {
				data := make([]byte, dataSize)
				_, err := f.ReadAt(data, int64(i*dataSize))
				if err != nil {
					t.Error(err)
				}
				if !bytes.Equal(data, datas[i]) {
					t.Error("data mismatch for value", i)
				}
			}
		}(i)
	}
	wg1.Wait()
}
