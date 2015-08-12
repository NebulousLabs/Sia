package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// testAllocation allocates and then deallocates a file, checking that the
// space is returned and the file is actually deleted.
func (ht *hostTester) testAllocation() {
	initialSpace := ht.host.spaceRemaining
	const filesize = 4e3

	// Allocate a 4kb file.
	file, path, err := ht.host.allocate(filesize)
	if err != nil {
		ht.t.Fatal(err)
	}
	file.Close()

	// Check that the file has a real name and that it exists on disk.
	fullpath := filepath.Join(ht.host.saveDir, path)
	_, err = os.Stat(fullpath)
	if os.IsNotExist(err) {
		ht.t.Fatal("file does not exist on disk")
	}

	// Check that spaceRemaining has decreased appropriately.
	if ht.host.spaceRemaining != initialSpace-filesize {
		ht.t.Error("space remaining did not decrease appropriately after allocating a file")
	}

	// Deallocate the file.
	ht.host.deallocate(filesize, path)
	if initialSpace != ht.host.spaceRemaining {
		ht.t.Error("space remaining did not return to the correct value after the file was deallocated")
	}
	_, err = os.Stat(fullpath)
	if !os.IsNotExist(err) {
		ht.t.Fatal("file still exists on disk after deallocation")
	}
}

// testConsiderTerms presents a sensible set of contract terms to the
// considerTerms function, and checks that they pass.
func (ht *hostTester) testConsiderTerms() {
	saneTerms := modules.ContractTerms{
		FileSize:      4e3,
		Duration:      12,
		DurationStart: 0,
		WindowSize:    ht.host.WindowSize,
		Price:         ht.host.Price,
		Collateral:    ht.host.Collateral,
		ValidProofOutputs: []types.SiacoinOutput{
			types.SiacoinOutput{
				UnlockHash: ht.host.UnlockHash,
			},
		},
		MissedProofOutputs: []types.SiacoinOutput{
			types.SiacoinOutput{
				UnlockHash: types.UnlockHash{},
			},
		},
	}

	err := ht.host.considerTerms(saneTerms)
	if err != nil {
		ht.t.Error(err)
	}
}

// TestAllocation creates a host tester and calls testAllocation.
func TestAllocation(t *testing.T) {
	ht := CreateHostTester("TestAllocation", t)
	ht.testAllocation()
}

// TestConsiderTerms creates a host tester and calls testConsiderTerms.
func TestConsiderTerms(t *testing.T) {
	ht := CreateHostTester("TestConsiderTerms", t)
	ht.testConsiderTerms()
}
