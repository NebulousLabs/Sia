package contractmanager

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
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

// TestNewContractManager does basic startup and shutdown of a contract
// manager, checking for egregious errors.
func TestNewContractManager(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

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

/*
import (
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
)

// storageManagerTester holds a testing-initialized storage manager and any
// additional fields that may be useful while testing.
type storageManagerTester struct {
	sm *StorageManager

	persistDir string
}

// addRandFolder connects a storage folder to a random directory in the
// tester's persist dir.
func (smt *storageManagerTester) addRandFolder(size uint64) error {
	dir := filepath.Join(smt.persistDir, persist.RandomSuffix())
	err := os.Mkdir(dir, 0700)
	if err != nil {
		return err
	}
	return smt.sm.AddStorageFolder(dir, size)
}

// capacity returns the amount of storage still available on the machine. The
// amount can be negative if the total capacity was reduced to below the active
// capacity.
func (sm *StorageManager) capacity() (total, remaining uint64) {
	// Total storage can be computed by summing the size of all the storage
	// folders.
	for _, sf := range sm.storageFolders {
		total += sf.Size
		remaining += sf.SizeRemaining
	}
	return total, remaining
}

// createSector makes a random, unique sector that can be inserted into the
// storage manager.
func createSector() (sectorRoot crypto.Hash, sectorData []byte, err error) {
	sectorData, err = crypto.RandBytes(int(modules.SectorSize))
	if err != nil {
		return crypto.Hash{}, nil, err
	}
	sectorRoot = crypto.MerkleRoot(sectorData)
	return sectorRoot, sectorData, nil
}

// newStorageManagerTester creates a storage tester ready for use.
func newStorageManagerTester(name string) (*storageManagerTester, error) {
	testdir := build.TempDir(modules.StorageManagerDir, name)
	sm, err := New(filepath.Join(testdir, modules.StorageManagerDir))
	if err != nil {
		return nil, err
	}
	smt := &storageManagerTester{
		sm: sm,

		persistDir: testdir,
	}
	return smt, nil
}

// probabilisticReset will probabilistically reboot the storage manager before
// continuing. This helps to verify that the persistence is working correctly.
// The reset is probabilistic to make sure that the test is not passing because
// of the reset.
func (smt *storageManagerTester) probabilisticReset() error {
	rand, err := crypto.RandIntn(3)
	if err != nil {
		return err
	}
	if rand == 1 {
		// Grab the potentially faulty dependencies and replace them with good
		// dependencies so that closing happens without issues.
		deps := smt.sm.dependencies
		smt.sm.dependencies = productionDependencies{}
		// Close the storage manager, then create a new storage manager to
		// replace it.
		err = smt.sm.Close()
		if err != nil {
			return err
		}
		// Open the storage manager with production dependencies so that there
		// are no errors.
		sm, err := New(filepath.Join(smt.persistDir, modules.StorageManagerDir))
		if err != nil {
			return err
		}
		sm.dependencies = deps
		smt.sm = sm
	}
	return nil
}

// Close shuts down all of the components of the storage manager tester.
func (smt *storageManagerTester) Close() error {
	return smt.sm.Close()
}
*/
