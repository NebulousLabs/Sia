package host

import (
	"os"
	"testing"
)

// testAllocation allocates and then deallocates a file, checking that the
// space is returned and the file is actually deleted.
func (ht *HostTester) testAllocation() {
	currentSpaceRemaining := ht.spaceRemaining

	// Allocate a 4kb file.
	file, path, err := ht.allocate(4e3)
	if err != nil {
		ht.Fatal(err)
	}
	file.Close()

	// Check that the file has a real name and that it exists on disk.
	if path == "" {
		ht.Fatal("path of allocated file is empty!")
	}
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		ht.Fatal("file does not exist on disk")
	}

	// Check that spaceRemaining has decreased appropriately.
	if currentSpaceRemaining != ht.spaceRemaining+4e3 {
		ht.Error("space remaining did not decrease appropriately after allocating a file")
	}

	// Deallocate the file.
	ht.deallocate(4e3, path)
	if currentSpaceRemaining != ht.spaceRemaining {
		ht.Error("space remaining did not return to the correct value after the file was deallocated")
	}
	_, err = os.Stat(path)
	if !os.IsNotExist(err) {
		ht.Fatal("file still exists on disk after deallocation")
	}
}

// TestAllocation creates a host tester and calls testAllocation.
func TestAllocation(t *testing.T) {
	ht := CreateHostTester(t)
	ht.testAllocation()
}
