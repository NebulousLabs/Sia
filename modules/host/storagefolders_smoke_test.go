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
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

// probabilisticReset will probabilistically reboot the host before continuing.
// This helps to verify that the persistence is working correctly. The reset is
// probabilistic to make sure that the test is not passing because of the
// reset.
func (ht *hostTester) probabilisticReset() error {
	rand, err := crypto.RandIntn(3)
	if err != nil {
		return err
	}
	if rand == 1 {
		// Grab the potentially faulty dependencies and replace them with good
		// dependencies so that closing happens without issues.
		deps := ht.host.dependencies
		ht.host.dependencies = productionDependencies{}
		// Close the host, then create a new host to replace it.
		err = ht.host.Close()
		if err != nil {
			return err
		}
		// Open the host with production dependencies so that there are no
		// errors.
		h, err := New(ht.cs, ht.tpool, ht.wallet, ":0", filepath.Join(ht.persistDir, modules.HostDir))
		if err != nil {
			return err
		}
		h.dependencies = deps
		ht.host = h
	}
	return nil
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
				return errors.New("usage information mismatch")
			}
			for i, expiryHeight := range usage.Expiry {
				if expiryHeight != expiryHeights[i] {
					// The correctness could be made not-implementation
					// dependent by sorting the two arrays before comparing
					// them, but that was deemed an unneeded step for this
					// test.
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
	t.Parallel()
	ht, err := newHostTester("TestStorageFolderUsage")
	if err != nil {
		t.Fatal(err)
	}
	// Remove the storage folders so that the smoke test can start from a
	// blank state.
	for i := 0; i < 2; i++ {
		err = ht.host.RemoveStorageFolder(0, false)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Start by checking that the initial state of the host has no storage
	// added to it.
	totalStorage, remainingStorage := ht.host.capacity()
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
	if err != errInsufficientStorageForSector {
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
	// Try a file size that is too large.
	err = ht.host.AddStorageFolder(storageFolderOne, maximumStorageFolderSize+1)
	if err != errLargeStorageFolder {
		t.Fatal("expecting errLargeStorageFolder:", err)
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
	// Do a probabilistic reset of the host, to verify that the persistence
	// structures can reboot without causing issues.
	err = ht.probabilisticReset()
	if err != nil {
		t.Fatal(err)
	}
	// Check that the host has correctly updated the amount of total storage.
	totalStorage, remainingStorage = ht.host.capacity()
	if totalStorage != minimumStorageFolderSize || remainingStorage != minimumStorageFolderSize {
		t.Error("host capacity has not been correctly updated after adding a storage folder")
		t.Error(totalStorage, minimumStorageFolderSize, remainingStorage)
	}

	// Add a second storage folder.
	storageFolderTwo := filepath.Join(ht.persistDir, "hostDrive2")
	err = os.Mkdir(storageFolderTwo, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.AddStorageFolder(storageFolderTwo, minimumStorageFolderSize*2)
	if err != nil {
		t.Fatal(err)
	}
	// Do a probabilistic reset of the host, to verify that the persistence
	// structures can reboot without causing issues.
	err = ht.probabilisticReset()
	if err != nil {
		t.Fatal(err)
	}
	// Check that the host has correctly updated the amount of total storage.
	totalStorage, remainingStorage = ht.host.capacity()
	if totalStorage != minimumStorageFolderSize*3 || remainingStorage != minimumStorageFolderSize*3 {
		t.Error("host capacity has not been correctly updated after adding a storage folder")
	}
	// Try removing the storage folder using illegal values.
	err = ht.host.RemoveStorageFolder(-1, false)
	if err != errBadStorageFolderIndex {
		t.Fatal(err)
	}
	err = ht.host.RemoveStorageFolder(2, false)
	if err != errBadStorageFolderIndex {
		t.Fatal(err)
	}

	// Try removing the second storage folder. Before removing the storage
	// folder, grab the path of the symlink so we can check later that it was
	// properly removed from the filesystem.
	ht.host.mu.Lock()
	symPath := filepath.Join(ht.host.persistDir, ht.host.storageFolders[1].uidString())
	ht.host.mu.Unlock()
	// Remove the storage folder.
	err = ht.host.RemoveStorageFolder(1, false)
	if err != nil {
		t.Fatal(err)
	}
	// Do a probabilistic reset of the host, to verify that the persistence
	// structures can reboot without causing issues.
	err = ht.probabilisticReset()
	if err != nil {
		t.Fatal(err)
	}
	// Check that the host has correctly updated the amount of total storage.
	totalStorage, remainingStorage = ht.host.capacity()
	if totalStorage != minimumStorageFolderSize || remainingStorage != minimumStorageFolderSize {
		t.Error("host capacity has not been correctly updated after adding a storage folder")
	}
	_, err = os.Stat(symPath)
	if err == nil || !os.IsNotExist(err) {
		t.Error("Does not appear that the sympath was removed from disk:", err)
	}

	// No sectors added yet, the storage folder statistics should all be clean.
	for _, sf := range ht.host.storageFolders {
		if sf.SuccessfulReads != 0 || sf.SuccessfulWrites != 0 || sf.FailedReads != 0 || sf.FailedWrites != 0 {
			t.Error("storage folder does not have blank health stats")
		}
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
	totalStorage, remainingStorage = ht.host.capacity()
	if totalStorage != minimumStorageFolderSize || remainingStorage != minimumStorageFolderSize-modules.SectorSize {
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
		if uint64(fileInfo.Size()) != modules.SectorSize {
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
	// Check that the disk health stats match the expected values.
	for _, sf := range ht.host.storageFolders {
		if sf.SuccessfulReads != 0 || sf.SuccessfulWrites != 1 || sf.FailedReads != 0 || sf.FailedWrites != 0 {
			t.Error("storage folder does not have blank health stats")
		}
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
	err = ht.host.ResizeStorageFolder(0, maximumStorageFolderSize+1)
	if err != errLargeStorageFolder {
		t.Error(err)
	}
	err = ht.host.ResizeStorageFolder(0, minimumStorageFolderSize*10)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.ResizeStorageFolder(0, minimumStorageFolderSize*10)
	if err != errNoResize {
		t.Fatal(err)
	}
	// Do a probabilistic reset of the host, to verify that the persistence
	// structures can reboot without causing issues.
	err = ht.probabilisticReset()
	if err != nil {
		t.Fatal(err)
	}
	// Host should be able to support having uneven storage sizes.
	oddStorageSize := (minimumStorageFolderSize) + modules.SectorSize*3 + 3
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
	totalStorage, remainingStorage = ht.host.capacity()
	if totalStorage != remainingStorage+modules.SectorSize {
		t.Fatal("host is not empty at the moment of creating the in-memory sector usage map")
	}
	// Verify that the inital sector usage map was created correctly.
	err = ht.sectorUsageCheck(sectorUsageMap)
	if err != nil {
		t.Fatal(err)
	}

	// Fill the storage folder above the minimum size, then try to shrink it to
	// the minimum size.
	for i := uint64(0); i <= minimumStorageFolderSize/modules.SectorSize; i++ {
		sectorRoot, sectorData, err := createSector()
		if err != nil {
			t.Fatal(err)
		}
		err = ht.host.addSector(sectorRoot, 86+types.BlockHeight(i), sectorData)
		if err != nil {
			t.Fatal(err)
		}
		// Do a probabilistic reset of the host, to verify that the persistence
		// structures can reboot without causing issues.
		err = ht.probabilisticReset()
		if err != nil {
			t.Fatal(err)
		}
		// Now that there is a sector usage map, it must be kept consistent
		// with the sector usage in the host.
		sectorUsageMap[sectorRoot] = []types.BlockHeight{86 + types.BlockHeight(i)}
	}
	ht.host.mu.Lock()
	oldSize := ht.host.storageFolders[0].Size
	ht.host.mu.Unlock()
	err = ht.host.ResizeStorageFolder(0, minimumStorageFolderSize)
	if err != errIncompleteOffload {
		t.Fatal(err)
	}
	ht.host.mu.Lock()
	size := ht.host.storageFolders[0].Size
	sizeRemaining := ht.host.storageFolders[0].SizeRemaining
	ht.host.mu.Unlock()
	if size >= oldSize || sizeRemaining > 0 {
		t.Fatal("host did not correctly update the size remaining after an incomplete shrink")
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
	// Do a probabilistic reset of the host, to verify that the persistence
	// structures can reboot without causing issues.
	err = ht.probabilisticReset()
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
	if folderTwoUsage != modules.SectorSize {
		t.Error("sector did not appear to land in the right storage folder")
	}
	// Check the filesystem. The folder for storage folder 1 should have 10
	// files, and the folder for storage folder 2 should have 1 file.
	infos, err := ioutil.ReadDir(storageFolderOne)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 10 {
		t.Fatal("storage folder one should have 10 sectors in it")
	}
	infos, err = ioutil.ReadDir(storageFolderTwo)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 {
		t.Fatal("storage folder two should have 1 sector in it")
	}

	// The first storage folder has more sectors than the minimum allowed
	// amount. Reduce the size of the first storage folder to minimum, which
	// should be accepted but will result in sectors being transferred to the
	// second storage folder.
	totalStorage, remainingStorage = ht.host.capacity()
	prevStorage := totalStorage
	usedStorage := totalStorage - remainingStorage
	err = ht.host.ResizeStorageFolder(0, minimumStorageFolderSize)
	if err != nil {
		t.Fatal(err)
	}
	totalStorage, remainingStorage = ht.host.capacity()
	if usedStorage != totalStorage-remainingStorage {
		t.Error("the used storage value adjusted after removing a storage folder", usedStorage, totalStorage-remainingStorage)
	}
	if totalStorage >= prevStorage {
		t.Error("total storage was not adjusted correctly after removing a storage folder")
	}
	// Check the filesystem.
	infos, err = ioutil.ReadDir(storageFolderOne)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 8 {
		t.Fatal("wrong number of sectors in storage folder one")
	}
	infos, err = ioutil.ReadDir(storageFolderTwo)
	if len(infos) != 3 {
		t.Fatal("wrong number of sectors in storage folder two")
	}

	// Remove the first storage folder, which should result in all of the
	// sectors being moved to the second storage folder. Note that
	// storageFolderTwo now has an index of '0'.
	totalStorage, remainingStorage = ht.host.capacity()
	prevStorage = totalStorage
	usedStorage = totalStorage - remainingStorage
	ht.host.mu.Lock()
	symPath = filepath.Join(ht.host.persistDir, ht.host.storageFolders[0].uidString())
	ht.host.mu.Unlock()
	err = ht.host.RemoveStorageFolder(0, false)
	if err != nil {
		t.Fatal(err)
	}
	totalStorage, remainingStorage = ht.host.capacity()
	if usedStorage != totalStorage-remainingStorage {
		t.Error("the used storage value adjusted after removing a storage folder", usedStorage, totalStorage-remainingStorage)
	}
	if totalStorage == prevStorage {
		t.Error("total storage was not adjusted after removing a storage folder")
	}
	// Check that the filesystem seems correct.
	infos, err = ioutil.ReadDir(storageFolderTwo)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 11 {
		t.Fatal("wrong number of sectors in folder")
	}
	_, err = os.Stat(symPath)
	if !os.IsNotExist(err) {
		t.Fatal("the sym link to the deleted storage folder should no longer exist")
	}

	// Add the first storage folder, resize the second storage folder back down
	// to minimum. Note that storageFolderOne now has an index of '1', and
	// storageFolderTwo now has an index of '0'.
	err = ht.host.AddStorageFolder(storageFolderOne, minimumStorageFolderSize)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.ResizeStorageFolder(0, minimumStorageFolderSize)
	if err != nil {
		t.Fatal(err)
	}
	// Do a probabilistic reset of the host, to verify that the persistence
	// structures can reboot without causing issues.
	err = ht.probabilisticReset()
	if err != nil {
		t.Fatal(err)
	}
	// Check the filesystem.
	infos, err = ioutil.ReadDir(storageFolderTwo)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 8 {
		t.Fatal("wrong number of sectors")
	}
	infos, err = ioutil.ReadDir(storageFolderOne)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 3 {
		t.Fatal("wrong number of sectors")
	}

	// Add a bunch of sectors and repeat sectors at multiple colliding heights.
	// Start by resizing the first storage folder so that there is enough room
	// for the new sectors.
	err = ht.host.ResizeStorageFolder(0, minimumStorageFolderSize*3)
	if err != nil {
		t.Fatal(err)
	}
	for i := types.BlockHeight(0); i < 10; i++ {
		// Add 10 unique sectors to the map.
		sectorRoot, sectorData, err := createSector()
		if err != nil {
			t.Fatal(err)
		}
		for j := types.BlockHeight(0); j < 5; j++ {
			// Add the unique sectors at multiple heights, creating virtual
			// sectors.
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
			// Do a probabilistic reset of the host, to verify that the persistence
			// structures can reboot without causing issues.
			err = ht.probabilisticReset()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	// Check that the amount of storage in use represents 10 sectors, and not
	// more - all the others are repeats and shouldn't be counted.
	totalStorage, remainingStorage = ht.host.capacity()
	if totalStorage != minimumStorageFolderSize*4 || remainingStorage != minimumStorageFolderSize*4-modules.SectorSize*21 {
		t.Fatal("Host not reporting expected storage capacity:", totalStorage, remainingStorage, minimumStorageFolderSize*4, minimumStorageFolderSize*4-modules.SectorSize*21)
	}
	// Check that the internal sector usage database of the host has been
	// updated correctly.
	err = ht.sectorUsageCheck(sectorUsageMap)
	if err != nil {
		t.Fatal(err)
	}
	// Check the filesystem.
	infos, err = ioutil.ReadDir(storageFolderTwo)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 16 {
		t.Fatal("there should be 16 sectors in storage folder two")
	}
	infos, err = ioutil.ReadDir(storageFolderOne)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 5 {
		t.Fatal("there should be 5 sectors in storage folder one")
	}
	// Try removing a non-repeat sector.
	expiryHeights, exists := sectorUsageMap[sectorRoot]
	if !exists || len(expiryHeights) != 1 {
		t.Fatal("sector map doesn't match testing assumptions")
	}
	// Try some illegal sector removal operations before trying a legal one.
	ht.host.mu.Lock()
	err = ht.host.removeSector(sectorRoot, sectorExpiry+50e6)
	ht.host.mu.Unlock()
	if err != errSectorNotFound {
		t.Fatal("wrong error when removing illegal sector:", err)
	}
	alteredRoot := sectorRoot
	alteredRoot[0]++
	ht.host.mu.Lock()
	err = ht.host.removeSector(alteredRoot, 81)
	ht.host.mu.Unlock()
	if err != errSectorNotFound {
		t.Fatal("wrong error when removing illegal sector:", err)
	}
	// Now try the legal sector removal.
	ht.host.mu.Lock()
	sectorPath = filepath.Join(storageFolderOne, string(ht.host.sectorID(sectorRoot[:])))
	err = ht.host.removeSector(sectorRoot, 81)
	ht.host.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	// Do a probabilistic reset of the host, to verify that the persistence
	// structures can reboot without causing issues.
	err = ht.probabilisticReset()
	if err != nil {
		t.Fatal(err)
	}
	// Update the sector usage map to reflect the departure of a sector.
	delete(sectorUsageMap, sectorRoot)
	// Check that the new capacity is being reported correctly.
	totalStorage, remainingStorage = ht.host.capacity()
	if totalStorage != minimumStorageFolderSize*4 || remainingStorage != minimumStorageFolderSize*4-modules.SectorSize*20 {
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
	// Check that the total number of sectors seen on disk is 20.
	infos, err = ioutil.ReadDir(storageFolderOne)
	if err != nil {
		t.Fatal(err)
	}
	infos2, err := ioutil.ReadDir(storageFolderTwo)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos)+len(infos2) != 20 {
		t.Fatal("there should be 20 sectors total on disk at this point")
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
	for i, root := range targetedRoots {
		// Grab the initial remaining storage, to make sure that it's not being
		// changed when one instance of a repeated sector is removed.
		_, initialRemainingStorage := ht.host.capacity()

		// Remove the heights one at a time.
		expiryHeights := sectorUsageMap[root]
		for len(expiryHeights) > 0 {
			// Check that the remaining storage is still the same.
			_, remainingStorage := ht.host.capacity()
			if remainingStorage != initialRemainingStorage {
				t.Fatal("host is changing the amount of storage remaining when removing virtual sectors")
			}

			// Try to remove the sector using a wildcard expiry height.
			ht.host.mu.Lock()
			err = ht.host.removeSector(root, expiryHeights[0]+548e6)
			ht.host.mu.Unlock()
			if err != errSectorNotFound {
				t.Fatal(err)
			}

			// Remove the sector from the host.
			ht.host.mu.Lock()
			err = ht.host.removeSector(root, expiryHeights[0])
			ht.host.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}

			// Check that the filesystem is housing the correct number of
			// sectors.
			infos, err = ioutil.ReadDir(storageFolderOne)
			if err != nil {
				t.Fatal(err)
			}
			infos2, err = ioutil.ReadDir(storageFolderTwo)
			if err != nil {
				t.Fatal(err)
			}
			bonus := 0
			if len(expiryHeights) == 1 {
				// If this is the last expiry height, the sector is no longer
				// viritual and is being removed for real, so we need to
				// subtract it from the expected total number of sectors.
				bonus++
			}
			if len(infos)+len(infos2) != 20-i-bonus {
				t.Fatal("sector count is incorrect while managing virtual sectors")
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
		// Do a probabilistic reset of the host, to verify that the persistence
		// structures can reboot without causing issues.
		err = ht.probabilisticReset()
		if err != nil {
			t.Fatal(err)
		}
		// Check that the remaining storage is still the same.
		_, remainingStorage := ht.host.capacity()
		if remainingStorage != initialRemainingStorage+modules.SectorSize {
			t.Fatal("host incorrectly updated remaining space when deleting the final height for a sector")
		}
	}

	// Add a third storage folder.
	prevTotalStorage, prevRemainingStorage := ht.host.capacity()
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
	totalStorage, remainingStorage = ht.host.capacity()
	if totalStorage != prevTotalStorage+minimumStorageFolderSize*2 || remainingStorage != prevRemainingStorage+minimumStorageFolderSize*2 {
		t.Fatal("storage folder sizes are not being updated correctly when new storage folders are added")
	}

	// Add sectors until the storage folders have no more capacity.
	_, remainingStorage = ht.host.capacity()
	remainingSectors := remainingStorage / modules.SectorSize
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
	// Do a probabilistic reset of the host, to verify that the persistence
	// structures can reboot without causing issues.
	err = ht.probabilisticReset()
	if err != nil {
		t.Fatal(err)
	}
	_, remainingStorage = ht.host.capacity()
	if remainingStorage >= modules.SectorSize {
		t.Error("remaining storage is reporting incorrect result - should report that there is not enough room for another sector")
	}
	err = ht.sectorUsageCheck(sectorUsageMap)
	if err != nil {
		t.Fatal(err)
	}
	// Check the filesystem.
	infos, err = ioutil.ReadDir(storageFolderOne)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 8 {
		t.Fatal("expecting 8 sectors in storage folder one")
	}
	infos, err = ioutil.ReadDir(storageFolderTwo)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 24 {
		t.Fatal("expecting 24 sectors in storage folder two")
	}
	infos, err = ioutil.ReadDir(storageFolderThree)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 16 {
		t.Fatal("expecting 16 sectors in storage folder three")
	}

	// Do some resizing, to cause sectors to be moved around. Every storage
	// folder should have sectors that get moved off of it.
	err = ht.host.ResizeStorageFolder(1, minimumStorageFolderSize*6)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.ResizeStorageFolder(0, minimumStorageFolderSize)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.ResizeStorageFolder(2, minimumStorageFolderSize)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.ResizeStorageFolder(0, minimumStorageFolderSize*6)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.ResizeStorageFolder(1, minimumStorageFolderSize)
	if err != nil {
		t.Fatal(err)
	}
	// Check that all storage folders are reporting successful reads and
	// writes, with no failures.
	for _, sf := range ht.host.storageFolders {
		if sf.SuccessfulWrites <= 0 || sf.SuccessfulReads <= 0 || sf.FailedWrites > 0 || sf.FailedReads > 0 {
			t.Error("disk stats aren't making sense")
		}
	}

	// Remove all of the sectors.
	i := 0
	for sectorRoot, expiryHeights := range sectorUsageMap {
		// Grab the initial remaining storage, to make sure that it's not being
		// changed when one instance of a repeated sector is removed.
		_, initialRemainingStorage := ht.host.capacity()

		// Remove the heights one at a time.
		for j := range expiryHeights {
			// Check that the remaining storage is still the same.
			_, remainingStorage := ht.host.capacity()
			if remainingStorage != initialRemainingStorage {
				t.Fatal("host is changing the amount of storage remaining when removing virtual sectors")
			}

			// Remove the sector from the host.
			ht.host.mu.Lock()
			err = ht.host.removeSector(sectorRoot, expiryHeights[j])
			ht.host.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}

			// Check that the filesystem is housing the correct number of
			// sectors.
			infos, err := ioutil.ReadDir(storageFolderOne)
			if err != nil {
				t.Fatal(err)
			}
			infos2, err := ioutil.ReadDir(storageFolderTwo)
			if err != nil {
				t.Fatal(err)
			}
			infos3, err := ioutil.ReadDir(storageFolderThree)
			if err != nil {
				t.Fatal(err)
			}
			bonus := 0
			if j == len(expiryHeights)-1 {
				// If this is the last expiry height, the sector is no longer
				// viritual and is being removed for real, so we need to
				// subtract it from the expected total number of sectors.
				bonus++
			}
			if len(infos)+len(infos2)+len(infos3) != 48-i-bonus {
				t.Error(len(infos)+len(infos2)+len(infos3), i, bonus)
				t.Fatal("sector count is incorrect while managing virtual sectors")
			}
		}
		// Do a probabilistic reset of the host, to verify that the persistence
		// structures can reboot without causing issues.
		err = ht.probabilisticReset()
		if err != nil {
			t.Fatal(err)
		}
		// Check that the remaining storage is still the same.
		_, remainingStorage := ht.host.capacity()
		if remainingStorage != initialRemainingStorage+modules.SectorSize {
			t.Fatal("host incorrectly updated remaining space when deleting the final height for a sector")
		}
		i++
	}
	// Check that all storage folders have successful writes, and no failed
	// reads or writes.
	for _, sf := range ht.host.storageFolders {
		if sf.SuccessfulWrites <= 0 || sf.SuccessfulReads <= 0 || sf.FailedWrites > 0 || sf.FailedReads > 0 {
			t.Error("disk stats aren't making sense")
		}
	}

	// Remove all of the storage folders.
	for i := 0; i < 3; i++ {
		err = ht.host.RemoveStorageFolder(0, false)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Check the filesystem, there should be 3 files in the host folder
	// (host.db, host.json, host.log).
	infos, err = ioutil.ReadDir(ht.host.persistDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 3 {
		t.Error("unexpected number of files in the host directory")
	}
}
