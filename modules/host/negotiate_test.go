package host

import (
	"os"
	"path/filepath"
	"testing"
)

// testAllocation allocates and then deallocates a file, checking that the
// file is actually deleted.
func (ht *hostTester) testAllocation() {
	// Allocate a file.
	file, path, err := ht.host.allocate()
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

	// Deallocate the file.
	ht.host.deallocate(path)
	_, err = os.Stat(fullpath)
	if !os.IsNotExist(err) {
		ht.t.Fatal("file still exists on disk after deallocation")
	}
}

// testConsiderTerms presents a sensible set of contract terms to the
// considerTerms function, and checks that they pass.
func (ht *hostTester) testConsiderTerms() {
	ht.t.Skip("TODO: replace with testConsiderTransaction and testConsiderRevision")
	/*
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
	*/
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
