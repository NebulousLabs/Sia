package contractmanager

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
