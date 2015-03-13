package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
)

// testAllocation allocates and then deallocates a file, checking that the
// space is returned and the file is actually deleted.
func (ht *HostTester) testAllocation() {
	initialSpace := ht.spaceRemaining
	const filesize = 4e3

	// Allocate a 4kb file.
	file, path, err := ht.allocate(filesize)
	if err != nil {
		ht.Fatal(err)
	}
	file.Close()

	// Check that the file has a real name and that it exists on disk.
	fullpath := filepath.Join(ht.Host.saveDir, path)
	_, err = os.Stat(fullpath)
	if os.IsNotExist(err) {
		ht.Fatal("file does not exist on disk")
	}

	// Check that spaceRemaining has decreased appropriately.
	if ht.spaceRemaining != initialSpace-filesize {
		ht.Error("space remaining did not decrease appropriately after allocating a file")
	}

	// Deallocate the file.
	ht.deallocate(filesize, path)
	if initialSpace != ht.spaceRemaining {
		ht.Error("space remaining did not return to the correct value after the file was deallocated")
	}
	_, err = os.Stat(fullpath)
	if !os.IsNotExist(err) {
		ht.Fatal("file still exists on disk after deallocation")
	}
}

// testConsiderTerms presents a sensible set of contract terms to the
// considerTerms function, and checks that they pass.
func (ht *HostTester) testConsiderTerms() {
	saneTerms := modules.ContractTerms{
		FileSize:      4e3,
		Duration:      12,
		DurationStart: 0,
		WindowSize:    ht.WindowSize,
		Price:         ht.Price,
		Collateral:    ht.Collateral,
		ValidProofOutputs: []consensus.SiacoinOutput{
			consensus.SiacoinOutput{
				UnlockHash: ht.Host.UnlockHash,
			},
		},
		MissedProofOutputs: []consensus.SiacoinOutput{
			consensus.SiacoinOutput{
				UnlockHash: consensus.ZeroUnlockHash,
			},
		},
	}

	err := ht.considerTerms(saneTerms)
	if err != nil {
		ht.Error(err)
	}
}

// TestAllocation creates a host tester and calls testAllocation.
func TestAllocation(t *testing.T) {
	ht := CreateHostTester(t)
	ht.testAllocation()
}

// TestConsiderTerms creates a host tester and calls testConsiderTerms.
func TestConsiderTerms(t *testing.T) {
	ht := CreateHostTester(t)
	ht.testConsiderTerms()
}
