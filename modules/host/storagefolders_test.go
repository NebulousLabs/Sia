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

// createSector makes a random, unique sector that can be inserted into the
// host.
func createSector() (sectorRoot crypto.Hash, sectorData []byte, err error) {
	sectorData, err = crypto.RandBytes(int(sectorSize))
	if err != nil {
		return crypto.Hash{}, nil, err
	}
	sectorRoot, err = crypto.ReaderMerkleRoot(bytes.NewReader(sectorData))
	if err != nil {
		return crypto.Hash{}, nil, err
	}
	return sectorRoot, sectorData, nil
}

// sectorUsageCheck compares a manually maintained sector usage map to the
// host's internal sector usage map, and returns an error if there are any
// inconsistencies.
func (ht *hostTester) sectorUsageCheck(sectorUsageMap map[crypto.Hash][]types.BlockHeight) error {
	// Check that the in-database representation for the sector usage map
	// matches the in-memory understanding of what the sector map should be
	return ht.host.db.View(func(tx *bolt.Tx) error {
		bsu := tx.Bucket(bucketSectorUsage)
		// Make sure that the number of sectors in the sector usage map and the
		// number of sectors in the database are the same.
		if len(sectorUsageMap) != bsu.Stats().KeyN {
			return errors.New("BucketSectorUsage has the wrong number of sectors recorded")
		}

		// For every sector in the sector usage map, make sure the database has
		// a matching sector with the right expiry information.
		for sectorRoot, expiryHeights := range sectorUsageMap {
			usageBytes := bsu.Get(ht.host.sectorID(sectorRoot[:]))
			if usageBytes == nil {
				return errors.New("no usage info on known sector")
			}
			var usage sectorUsage
			err := json.Unmarshal(usageBytes, &usage)
			if err != nil {
				return err
			}
			if len(usage.Expiry) != len(expiryHeights) {
				println(len(usage.Expiry))
				println(len(expiryHeights))
				println(string(ht.host.sectorID(sectorRoot[:])))
				return errors.New("usage information mismatch")
			}
			for i, expiryHeight := range usage.Expiry {
				if expiryHeight != expiryHeights[i] {
					// The correctness could be made not-implementation
					// dependent by sorting the two arrays before comparing
					// them, but that was deemed an unneeded step for this
					// test.
					println(expiryHeight)
					println(expiryHeights[i])
					return errors.New("usage expiry height mismatch - correctness is implementation dependent")
				}
			}
		}
		return nil
	})
}

// TestStorageFolderUsage is a general integration test which tries all of the
// major storage folder operations in various orders, all while adding and
// removing sectors to verify that the behavior works as expected.
func TestStorageFolderUsage(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
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
	// Host needs to be locked when the unexported sector function is being
	// used.
	sectorRoot, sectorData, err := createSector()
	if err != nil {
		t.Fatal(err)
	}
	ht.host.mu.Lock()
	err = ht.host.addSector(sectorRoot, 10, sectorData)
	ht.host.mu.Unlock()
	if err != errNoStorage {
		t.Fatal(err)
	}

	// Add a storage folder, simulating a new drive being connected to the
	// host.
	storageFolderOne := filepath.Join(ht.persistDir, "host drive 1")
	// Try using a file size that is too small. Because a filesize check is
	// quicker than a disk check, the filesize check should come first.
	err = ht.host.AddStorageFolder(storageFolderOne, minimumStorageFolderSize-1)
	if err != errSmallStorageFolder {
		t.Fatal("expecting errSmallStorageFolder:", err)
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
	if err != errStorageFolderNotFolder {
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
		t.Error(totalStorage, minimumStorageFolderSize, remainingStorage)
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
	// Try removing the storage folder using illegal values.
	err = ht.host.RemoveStorageFolder(-1)
	if err != errBadStorageFolderIndex {
		t.Fatal(err)
	}
	err = ht.host.RemoveStorageFolder(2)
	if err != errBadStorageFolderIndex {
		t.Fatal(err)
	}

	// Before removing the storage folder, grab the path of the symlink so we
	// can check later that it was properly removed from the filesystem.
	ht.host.mu.Lock()
	symPath := filepath.Join(ht.host.persistDir, ht.host.storageFolders[1].uidString())
	ht.host.mu.Unlock()
	// Remove the storage folder.
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
	_, err = os.Stat(symPath)
	if err == nil || !os.IsNotExist(err) {
		t.Error("Does not appear that the sympath was removed from disk:", err)
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
	sectorPath := filepath.Join(storageFolderOne, string(ht.host.sectorID(sectorRoot[:])))
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
		bsu := tx.Bucket(bucketSectorUsage)
		usageBytes := bsu.Get(ht.host.sectorID(sectorRoot[:]))
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
	err = ht.host.ResizeStorageFolder(1, minimumStorageFolderSize-1)
	if err != errBadStorageFolderIndex {
		t.Error(err)
	}
	err = ht.host.ResizeStorageFolder(-1, minimumStorageFolderSize-1)
	if err != errBadStorageFolderIndex {
		t.Error(err)
	}
	err = ht.host.ResizeStorageFolder(0, minimumStorageFolderSize-1)
	if err != errSmallStorageFolder {
		t.Error(err)
	}
	err = ht.host.ResizeStorageFolder(0, minimumStorageFolderSize*100)
	if err != nil {
		t.Fatal(err)
	}
	// Host should be able to support having uneven storage sizes.
	oddStorageSize := (minimumStorageFolderSize) + sectorSize*2 + 3
	err = ht.host.ResizeStorageFolder(0, oddStorageSize)
	if err != nil {
		t.Fatal(err)
	}

	// Create a sector list, containing all sectors (including repeats) and the
	// heights at which they expire. This sector list will be updated as
	// sectors are added and removed.
	sectorUsageMap := make(map[crypto.Hash][]types.BlockHeight)
	sectorUsageMap[sectorRoot] = []types.BlockHeight{sectorExpiry}
	// Sanity check - host should not have any sectors in it.
	totalStorage, remainingStorage, err = ht.host.Capacity()
	if err != nil {
		t.Fatal(err)
	}
	if totalStorage != remainingStorage+sectorSize {
		t.Fatal("host is not empty at the moment of creating the in-memory sector usage map")
	}

	// Fill the storage folder above the minimum size, then try to shrink it to
	// the minimum size.
	for i := uint64(0); i <= minimumStorageFolderSize/sectorSize; i++ {
		sectorRoot, sectorData, err := createSector()
		if err != nil {
			t.Fatal(err)
		}
		err = ht.host.addSector(sectorRoot, 86, sectorData)
		if err != nil {
			t.Fatal(err)
		}
		// Now that there is a sector usage map, it must be kept consistent
		// with the sector usage in the host.
		sectorUsageMap[sectorRoot] = []types.BlockHeight{86}
	}
	err = ht.host.ResizeStorageFolder(0, minimumStorageFolderSize)
	if err != errInsufficientRemainingStorageForShrink {
		t.Fatal(err)
	}
	// Try adding another sector, there should not be enough room.
	sr, sd, err := createSector()
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.addSector(sr, 186, sd)
	if err != errInsufficientStorageForSector {
		t.Fatal(err)
	}

	// Add a second folder, and add a sector to that folder. There should be
	// enough space remaining in the first folder for the removal to be
	// successful.
	err = ht.host.AddStorageFolder(storageFolderTwo, minimumStorageFolderSize*2)
	if err != nil {
		t.Fatal(err)
	}
	sectorRoot, sectorData, err = createSector()
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.addSector(sectorRoot, 81, sectorData)
	if err != nil {
		t.Fatal(err)
	}
	sectorUsageMap[sectorRoot] = []types.BlockHeight{81}
	// Check that the sector ended up in the right storage folder - because the
	// second storage folder is the least full, the sector should end up there.
	ht.host.mu.Lock()
	folderTwoUsage := ht.host.storageFolders[1].Size - ht.host.storageFolders[1].SizeRemaining
	ht.host.mu.Unlock()
	if folderTwoUsage != sectorSize {
		t.Error("sector did not appear to land in the right storage folder")
	}
	// TODO: Check the filesystem appearance. (passes visual check, but needs
	// an automatic check too)

	// The first storage folder has more sectors than the minimum allowed
	// amount. Reduce the size of the first storage folder to minimum, which
	// should be accepted but will result in sectors being transferred to the
	// second storage folder.
	totalStorage, remainingStorage, err = ht.host.Capacity()
	if err != nil {
		t.Fatal(err)
	}
	prevStorage := totalStorage
	usedStorage := totalStorage - remainingStorage
	err = ht.host.RemoveStorageFolder(0)
	if err != nil {
		t.Fatal(err)
	}
	totalStorage, remainingStorage, err = ht.host.Capacity()
	if err != nil {
		t.Fatal(err)
	}
	if usedStorage != totalStorage-remainingStorage {
		t.Error("the used storage value adjusted after removing a storage folder", usedStorage, totalStorage-remainingStorage)
	}
	if totalStorage == prevStorage {
		t.Error("total storage was not adjusted after removing a storage folder")
	}
	// TODO: Check the filesystem. (visual check holds up)

	// Add the first storage folder, resize the second storage folder back down
	// to minimum. Because of the naming, storage folder two is now actually
	// storage folder one.
	err = ht.host.AddStorageFolder(storageFolderOne, minimumStorageFolderSize)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.ResizeStorageFolder(0, minimumStorageFolderSize*3)
	if err != nil {
		t.Fatal(err)
	}
	// TODO: Check the filesystem. (visual check holds up)

	// Add a bunch of sectors and repeat sectors at multiple colliding heights.
	for i := types.BlockHeight(0); i < 10; i++ {
		// Add 10 unique sectors to the map.
		sectorRoot, sectorData, err := createSector()
		if err != nil {
			t.Fatal(err)
		}
		for j := types.BlockHeight(0); j < 5; j++ {
			// Add the unique sectors at multiple heights.
			for k := types.BlockHeight(0); k < 4; k++ {
				// Add in an extra loop so that height collisions can be
				// created such that the collisions happen out of order.
				// Sectors are added at height 10+j+k, which means that there
				// will be many collisions for each height, but the collisions
				// are not happening in sorted order. The host is not expected
				// to do sorting, but should also not be confused by a random
				// order.
				err = ht.host.addSector(sectorRoot, 10+j+k, sectorData)
				if err != nil {
					t.Fatal(err)
				}

				// Add the sector to the sectorUsageMap, so it can be deleted
				// later.
				expiryList, exists := sectorUsageMap[sectorRoot]
				if exists {
					sectorUsageMap[sectorRoot] = append(expiryList, 10+j+k)
				} else {
					sectorUsageMap[sectorRoot] = []types.BlockHeight{10 + j + k}
				}
			}
		}
	}
	// Check that the amount of storage in use represents 10 sectors, and not
	// more - all the others are repeats and shouldn't be counted.
	totalStorage, remainingStorage, err = ht.host.Capacity()
	if err != nil {
		t.Fatal(err)
	}
	if totalStorage != minimumStorageFolderSize*4 || remainingStorage != minimumStorageFolderSize*4-sectorSize*21 {
		t.Fatal("Host not reporting expected storage capacity:", totalStorage, remainingStorage, minimumStorageFolderSize*4, minimumStorageFolderSize*4-sectorSize*21)
	}
	// Check that the internal sector usage database of the host has been
	// updated correctly.
	err = ht.sectorUsageCheck(sectorUsageMap)
	if err != nil {
		t.Fatal(err)
	}
	// TODO: Check the filesystem (visual check holds up)

	// Try removing a non-repeat sector.
	expiryHeights, exists := sectorUsageMap[sectorRoot]
	if !exists || len(expiryHeights) != 1 {
		t.Fatal("sector map doesn't match testing assumptions")
	}
	ht.host.mu.Lock()
	err = ht.host.removeSector(sectorRoot, sectorExpiry)
	ht.host.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	// Update the sector usage map to reflect the departure of a sector.
	delete(sectorUsageMap, sectorRoot)
	// Check that the new capacity is being reported correctly.
	totalStorage, remainingStorage, err = ht.host.Capacity()
	if err != nil {
		t.Fatal(err)
	}
	if totalStorage != minimumStorageFolderSize*4 || remainingStorage != minimumStorageFolderSize*4-sectorSize*20 {
		t.Fatal("Host not reporting expected storage capacity:")
	}
	// Run a sector usage check to make sure the host is properly handling the
	// usage information when deleting a sector.
	err = ht.sectorUsageCheck(sectorUsageMap)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the sector on-disk has been deleted.
	_, err = os.Stat(sectorPath)
	if !os.IsNotExist(err) {
		t.Fatal(err)
	}

	// Remove two of the duplicated sectors, one copy at a time, to see that
	// the database is still updating correctly.
	var secondIteration bool
	var targetedRoots []crypto.Hash
	for sectorRoot, expiryHeights := range sectorUsageMap {
		if len(expiryHeights) < 2 {
			continue
		}
		targetedRoots = append(targetedRoots, sectorRoot)
		// Break on the second iteration.
		if secondIteration {
			break
		}
		secondIteration = true
	}
	// Remove, one piece at a time, the two targeted sectors.
	for _, root := range targetedRoots {
		// Grab the initial remaining storage, to make sure that it's not being
		// changed when one instance of a repeated sector is removed.
		_, initialRemainingStorage, err := ht.host.Capacity()
		if err != nil {
			t.Fatal(err)
		}

		// Remove the heights one at a time.
		expiryHeights := sectorUsageMap[root]
		for len(expiryHeights) > 0 {
			// Check that the remaining storage is still the same.
			_, remainingStorage, err := ht.host.Capacity()
			if err != nil {
				t.Fatal(err)
			}
			if remainingStorage != initialRemainingStorage {
				t.Fatal("host is changing the amount of storage remaining when removing virtual sectors")
			}

			// Remove the sector from the host.
			ht.host.mu.Lock()
			err = ht.host.removeSector(root, expiryHeights[0])
			ht.host.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}

			// Update the sector map to reflect the removed sector.
			if len(expiryHeights) > 1 {
				expiryHeights = expiryHeights[1:]
				sectorUsageMap[root] = expiryHeights
			} else {
				expiryHeights = nil
				delete(sectorUsageMap, root)
			}
			err = ht.sectorUsageCheck(sectorUsageMap)
			if err != nil {
				t.Fatal(err)
			}
		}
		// Check that the remaining storage is still the same.
		_, remainingStorage, err := ht.host.Capacity()
		if err != nil {
			t.Fatal(err)
		}
		if remainingStorage != initialRemainingStorage+sectorSize {
			t.Fatal("host incorrectly updated remaining space when deleting the final height for a sector")
		}
	}
	// TODO: Check the filesystem (visual check revealed no issues)

	// Add a third storage folder.
	prevTotalStorage, prevRemainingStorage, err := ht.host.Capacity()
	if err != nil {
		t.Fatal(err)
	}
	storageFolderThree := filepath.Join(ht.persistDir, "hd3")
	err = os.Mkdir(storageFolderThree, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.AddStorageFolder(storageFolderThree, minimumStorageFolderSize*2)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the total storage and remaining storage updated correctly.
	totalStorage, remainingStorage, err = ht.host.Capacity()
	if err != nil {
		t.Fatal(err)
	}
	if totalStorage != prevTotalStorage+minimumStorageFolderSize*2 || remainingStorage != prevRemainingStorage+minimumStorageFolderSize*2 {
		t.Fatal("storage folder sizes are not being updated correctly when new storage folders are added")
	}

	// Add sectors until the storage folders have no more capacity.
	_, remainingStorage, err = ht.host.Capacity()
	if err != nil {
		t.Fatal(err)
	}
	remainingSectors := remainingStorage / sectorSize
	for i := uint64(0); i < remainingSectors; i++ {
		sectorRoot, sectorData, err := createSector()
		if err != nil {
			t.Fatal(err)
		}
		ht.host.mu.Lock()
		err = ht.host.addSector(sectorRoot, 36, sectorData)
		ht.host.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		sectorUsageMap[sectorRoot] = []types.BlockHeight{36}
	}
	// Add another sector, which will not fit in the host.
	sectorRoot, sectorData, err = createSector()
	if err != nil {
		t.Fatal(err)
	}
	ht.host.mu.Lock()
	err = ht.host.addSector(sectorRoot, 36, sectorData)
	ht.host.mu.Unlock()
	if err != errInsufficientStorageForSector {
		t.Fatal(err)
	}
	_, remainingStorage, err = ht.host.Capacity()
	if err != nil {
		t.Fatal(err)
	}
	if remainingStorage >= sectorSize {
		t.Error("remaining storage is reporting incorrect result - should report that there is not enough room for another sector")
	}
	// TODO: Check the filesystem (visual check suggests it's all working)

	// TODO: Remove all of the sectors.

	// TODO: Remove all of the storage folders.

	// TODO: After you've done all of that, you need to add persistence-style
	// restarts between some of the major operations, to make sure that
	// persistence is correctly handling all of the field changes.

	// TODO: Somehow this stuff needs to be load-tested so we can see how a
	// host under duress will handle things.

	// TODO: Add some sanity checks to make sure that the host is not going
	// over its allocated total storage. This includes via obligations!

	// TODO: Have some conversion rate to deal with filesystem overhead, or
	// some other way of managing filesystem overhead, which really doesn't
	// seem like a predictable task.
}
