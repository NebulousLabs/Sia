package host

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

// TestStorageFolderUsage is a general integration test which tries all of the
// major storage folder operations in various orders, all while adding and
// removing sectors to verify that the behavior works as expected.
func TestStorageFolderUsage(t *testing.T) {
	if testing.Short() {
		// t.SkipNow()
	}
	ht, err := newHostTester("TestStorageFolderUsage")
	if err != nil {
		t.Fatal(err)
	}

	// Start by checking that the initial state of the host has no storage
	// added to it.
	totalStorage, remainingStorage, err := ht.host.Capacity()
	if err != nil {
		t.Fatal(err)
	}
	if totalStorage != 0 || remainingStorage != 0 {
		t.Error("initial capacity of host is not reported at 0 - but no drives have been added!")
	}

	// Try adding a sector when there are no storage folders.
	sectorData, err := crypto.RandBytes(int(sectorSize))
	if err != nil {
		t.Fatal(err)
	}
	sectorRoot, err := crypto.ReaderMerkleRoot(bytes.NewReader(sectorData))
	if err != nil {
		t.Fatal(err)
	}
	// Host needs to be locked when the unexported sector function is being
	// used.
	ht.host.mu.Lock()
	err = ht.host.addSector(sectorRoot, 10, sectorData)
	ht.host.mu.Unlock()
	if err != ErrInsufficientStorageForSector {
		t.Fatal(err)
	}

	// Add a storage folder, simulating a new drive being connected to the
	// host.
	storageFolderOne := filepath.Join(ht.persistDir, "host drive 1")
	// Try using a file size that is too small. Because a filesize check is
	// quicker than a disk check, the filesize check should come first.
	err = ht.host.AddStorageFolder(storageFolderOne, minimumStorageFolderSize-1)
	if err != ErrSmallStorageFolder {
		t.Fatal("expecting ErrSmallStorageFolder:", err)
	}
	// Try linking to a storage folder that does not exist.
	err = ht.host.AddStorageFolder(storageFolderOne, minimumStorageFolderSize)
	if err == nil {
		t.Fatal("should not be able to link to a storage folder which does not exist")
	}
	// Try linking to a storage folder that's not a directory.
	err = ioutil.WriteFile(storageFolderOne, make([]byte, minimumStorageFolderSize), 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.AddStorageFolder(storageFolderOne, minimumStorageFolderSize)
	if err != ErrStorageFolderNotFolder {
		t.Fatal(err)
	}
	// Try linking to a storage folder that is a directory.
	err = os.Remove(storageFolderOne)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Mkdir(storageFolderOne, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.AddStorageFolder(storageFolderOne, minimumStorageFolderSize)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the host has correctly updated the amount of total storage.
	totalStorage, remainingStorage, err = ht.host.Capacity()
	if err != nil {
		t.Fatal(err)
	}
	if totalStorage != minimumStorageFolderSize || remainingStorage != minimumStorageFolderSize {
		t.Error("host capacity has not been correctly updated after adding a storage folder")
	}

	// Add a second storage folder, and then remove it.
	storageFolderTwo := filepath.Join(ht.persistDir, "hostDrive2")
	err = os.Mkdir(storageFolderTwo, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.AddStorageFolder(storageFolderTwo, minimumStorageFolderSize*2)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the host has correctly updated the amount of total storage.
	totalStorage, remainingStorage, err = ht.host.Capacity()
	if err != nil {
		t.Fatal(err)
	}
	if totalStorage != minimumStorageFolderSize*3 || remainingStorage != minimumStorageFolderSize*3 {
		t.Error("host capacity has not been correctly updated after adding a storage folder")
	}
	err = ht.host.RemoveStorageFolder(1)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the host has correctly updated the amount of total storage.
	totalStorage, remainingStorage, err = ht.host.Capacity()
	if err != nil {
		t.Fatal(err)
	}
	if totalStorage != minimumStorageFolderSize || remainingStorage != minimumStorageFolderSize {
		t.Error("host capacity has not been correctly updated after adding a storage folder")
	}

	// Retry adding the sector, the add should succeed and the amount of
	// remaining storage should be updated.
	sectorExpiry := types.BlockHeight(10)
	ht.host.mu.Lock()
	err = ht.host.addSector(sectorRoot, sectorExpiry, sectorData)
	ht.host.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	// Check that the capacity has updated to reflected the new sector.
	totalStorage, remainingStorage, err = ht.host.Capacity()
	if err != nil {
		t.Fatal(err)
	}
	if totalStorage != minimumStorageFolderSize || remainingStorage != minimumStorageFolderSize-sectorSize {
		t.Error("host capacity has not been correctly updated after adding a sector", totalStorage, remainingStorage)
	}
	// Check that the sector has been added to the filesystem correctly - the
	// file should exist in storageFolderOne, and the data in the file should
	// match the data of the sector.
	sectorPath := filepath.Join(storageFolderOne, sectorRoot.String())
	err = func() error {
		sectorFile, err := os.Open(sectorPath)
		defer sectorFile.Close()
		fileInfo, err := sectorFile.Stat()
		if err != nil {
			return err
		}
		if uint64(fileInfo.Size()) != sectorSize {
			return errors.New("scanned sector is not the right size")
		}
		readSectorData, err := ioutil.ReadAll(sectorFile)
		if err != nil {
			return err
		}
		if bytes.Compare(readSectorData, sectorData) != 0 {
			return errors.New("read sector does not match sector data")
		}
		return nil
	}()
	if err != nil {
		t.Fatal(err)
	}
	// Check that the sector as represented in the database has the correct
	// height values.
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		bsu := tx.Bucket(BucketSectorUsage)
		usageBytes := bsu.Get(sectorRoot[:])
		var usage sectorUsage
		err := json.Unmarshal(usageBytes, &usage)
		if err != nil {
			return err
		}
		if len(usage.Expiry) != 1 {
			return errors.New("wrong usage expiry length in BucketSectorUsage")
		}
		if usage.Expiry[0] != 10 {
			return errors.New("usage expiry for sector is set to the wrong height")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Try to resize the storage folder. While resizing the storage folder, try
	// a bunch of invalid resize calls.
	err = ht.host.ResizeStorageFolder(1, 100e6)
	if err != ErrBadStorageFolderIndex {
		t.Error(err)
	}
	err = ht.host.ResizeStorageFolder(-1, 100e6)
	if err != ErrBadStorageFolderIndex {
		t.Error(err)
	}
	err = ht.host.ResizeStorageFolder(0, minimumStorageFolderSize-1)
	if err != ErrSmallStorageFolder {
		t.Error(err)
	}
	err = ht.host.ResizeStorageFolder(0, minimumStorageFolderSize*10)
	if err != nil {
		t.Fatal(err)
	}

	// Create a sector list, containing all sectors (including repeats) and the
	// heights at which they expire. This sector list will be updated as
	// sectors are added and removed.
	sectorUsageMap := make(map[crypto.Hash][]types.BlockHeight)
	sectorUsageMap[sectorRoot] = sectorExpiry

	// TODO: Add a bunch of sectors and repeat sectors at multiple colliding
	// heights.

	// TODO: Check that the in-database sector information is correct.

	// TODO: Add some non-repeating sectors to get variety.

	// TODO: Shrink the storage folder

	// TODO: Remove the repeat sectors bit by bit, making sure that the
	// database is updating correctly.

	// TODO: Remove a non-repeat sector.

	// TODO: Add another storage folder

	// TODO: Add multiple sectors, to the tune of filling up the storage
	// folders. Figure out what happens when you get full but keep trying to
	// add unique sectors.

	// TODO: Add a third storage folder.

	// TODO: Add a sector or two.

	// TODO: Add a fourth storage folder.

	// TODO: Add a sector or two.

	// TODO: Remove the second storage folder.

	// TODO: Increase the size of the third storage folder.

	// TODO: Shrink the fourth storage folder.

	// TODO: Remove all of the sectors.

	// TODO: Remove all of the storage folders.

	// TODO: After you've done all of that, you need to add persistence-style
	// restarts between some of the major operations, to make sure that
	// persistence is correctly handling all of the field changes.
}
