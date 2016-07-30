package contractmanager

/*
import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestMaxVirtualSectors checks that the max virtual sector limit is enforced
// when adding sectors.
func TestMaxVirtualSectors(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	smt, err := newStorageManagerTester("TestMaxVirtualSectors")
	if err != nil {
		t.Fatal(err)
	}
	defer smt.Close()

	// Add a storage folder to receive a sector.
	err = smt.sm.AddStorageFolder(smt.persistDir, minimumStorageFolderSize)
	if err != nil {
		t.Fatal(err)
	}

	// Add the first instance of the sector.
	sectorRoot, sectorData, err := createSector()
	if err != nil {
		t.Fatal(err)
	}
	err = smt.sm.AddSector(sectorRoot, 1, sectorData)
	if err != nil {
		t.Fatal(err)
	}

	// Add virtual instances of the sector until there are no more available
	// virual slots.
	for i := 1; i < maximumVirtualSectors; i++ {
		err = smt.sm.AddSector(sectorRoot, types.BlockHeight(i%3+2), sectorData)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Add another virtual sector, an error should be returned.
	err = smt.sm.AddSector(sectorRoot, 1, sectorData)
	if err != errMaxVirtualSectors {
		t.Fatal(err)
	}
}

// TestBadSectorAdd triggers a panic by trying to add an illegal sector.
func TestBadSectorAdd(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	smt, err := newStorageManagerTester("TestBadSectorAdd")
	if err != nil {
		t.Fatal(err)
	}
	defer smt.Close()
	// Add a storage folder to receive a sector.
	err = smt.sm.AddStorageFolder(smt.persistDir, minimumStorageFolderSize)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("did not trigger panic when adding a bad sector")
		}
	}()
	sectorRoot, sectorData, err := createSector()
	if err != nil {
		t.Fatal(err)
	}
	// Error doesn't need to be checked, a panic will be thrown.
	_ = smt.sm.AddSector(sectorRoot, 1, sectorData[:1])
	t.Fatal("panic not thrown")
}
*/
