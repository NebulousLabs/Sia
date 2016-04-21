package storagemanager

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

// storageManagerTester holds a testing-initialized storage manager and any
// additional fields that may be useful while testing.
type storageManagerTester struct {
	sm *StorageManager

	persistDir string
}

// createSector makes a random, unique sector that can be inserted into the
// host.
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

// Close shuts down all of the components of the storage manager tester.
func (smt *storageManagerTester) Close() error {
	return smt.sm.Close()
}
