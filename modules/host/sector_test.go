package host

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
	ht, err := newHostTester("TestMaxVirtualSectors")
	if err != nil {
		t.Fatal(err)
	}

	// Add a storage folder to receive a sector.
	err = ht.host.AddStorageFolder(ht.host.persistDir, minimumStorageFolderSize)
	if err != nil {
		t.Fatal(err)
	}

	// Add the first instance of the sector.
	sectorRoot, sectorData, err := createSector()
	if err != nil {
		t.Fatal(err)
	}
	ht.host.mu.Lock()
	err = ht.host.addSector(sectorRoot, 1, sectorData)
	ht.host.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	// Add virtual instances of the sector until there are no more available
	// virual slots.
	for i := 1; i < maximumVirtualSectors; i++ {
		ht.host.mu.Lock()
		err = ht.host.addSector(sectorRoot, types.BlockHeight(i%3+2), sectorData)
		ht.host.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Add another virtual sector, an error should be returned.
	ht.host.mu.Lock()
	err = ht.host.addSector(sectorRoot, 1, sectorData)
	ht.host.mu.Unlock()
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
	ht, err := newHostTester("TestBadSectorAdd")
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
	ht.host.mu.Lock()
	// Error doesn't need to be checked, a panic will be thrown.
	_ = ht.host.addSector(sectorRoot, 1, sectorData[:1])
	t.Fatal("panic not thrown")
}
