package contractmanager

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"gitlab.com/NebulousLabs/Sia/build"
	"gitlab.com/NebulousLabs/Sia/modules"
	"gitlab.com/NebulousLabs/fastrand"
)

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
	err := os.MkdirAll(testdir, 0700)
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
		fastrand.Read(datas[i])
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
