package contractmanager

/*
import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// faultyFS is a mocked filesystem which can be configured to fail for certain
// files and folders, as indicated by 'brokenSubstrings'.
type faultyFS struct {
	// brokenSubstrings is a list of substrings that, when appearing in a
	// filepath, will cause the call to fail.
	brokenSubstrings []string

	productionDependencies
}

// readFile reads a file from the filesystem. The call will fail if reading
// from a file that has a substring which matches the ffs list of broken
// substrings.
func (ffs faultyFS) readFile(s string) ([]byte, error) {
	for _, bs := range ffs.brokenSubstrings {
		if strings.Contains(s, bs) {
			return nil, mockErrReadFile
		}
	}
	return ffs.productionDependencies.readFile(s)
}

// symlink creates a symlink between a source and a destination file, but will
// fail if either filename contains a substring found in the set of broken
// substrings.
func (ffs faultyFS) symlink(s1, s2 string) error {
	for _, bs := range ffs.brokenSubstrings {
		if strings.Contains(s1, bs) || strings.Contains(s2, bs) {
			return mockErrSymlink
		}
	}
	return ffs.productionDependencies.symlink(s1, s2)
}

// writeFile reads a file from the filesystem. The call will fail if reading
// from a file that has a substring which matches the ffs list of broken
// substrings.
func (ffs faultyFS) writeFile(s string, b []byte, fm os.FileMode) error {
	// The partial write reqires that there be at least a few bytes, so that a
	// partial write can be properly simulated.
	if len(b) < 2 {
		panic("mocked writeFile requires file data that's at least 2 bytes in length")
	}

	for _, bs := range ffs.brokenSubstrings {
		if strings.Contains(s, bs) {
			// Do a partial write, so that garbase is left on the filesystem
			// that the code should be trying to clean up.
			err := ioutil.WriteFile(s, b[:len(b)/2], fm)
			if err != nil {
				return err
			}

			// Return a simulated failure, as the full slice was not written.
			return mockErrWriteFile
		}
	}
	return ioutil.WriteFile(s, b, fm)
}

// faultyRemove is a mocked set of dependencies that operates as normal except
// that removeFile will fail.
type faultyRemove struct {
	productionDependencies
}

// removeFile fails to remove a file from the filesystem.
func (faultyRemove) removeFile(s string) error {
	return mockErrRemoveFile
}

// TestStorageFolderTolerance tests the tolerance of storage folders in the
// presence of disk failures. Disk failures should be recorded, and the
// failures should be handled gracefully - nonfailing disks should not have
// problems.
func TestStorageFolderTolerance(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	smt, err := newStorageManagerTester("TestStorageFolderTolerance")
	if err != nil {
		t.Fatal(err)
	}
	defer smt.Close()

	// Replace the storage manager so that it's using faultyOS for its
	// dependencies.
	err = smt.sm.Close()
	if err != nil {
		t.Fatal(err)
	}
	ffs := new(faultyFS)
	smt.sm, err = newStorageManager(ffs, filepath.Join(smt.persistDir, modules.StorageManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Add a storage folder when the symlinking is failing.
	storageFolderOne := filepath.Join(smt.persistDir, "driveOne")
	ffs.brokenSubstrings = []string{storageFolderOne}
	err = os.Mkdir(storageFolderOne, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = smt.sm.AddStorageFolder(storageFolderOne, minimumStorageFolderSize)
	if err != mockErrSymlink {
		t.Fatal(err)
	}

	// Add storage folder one without errors, and then add a sector to the
	// storage folder.
	ffs.brokenSubstrings = nil
	err = smt.sm.AddStorageFolder(storageFolderOne, minimumStorageFolderSize)
	if err != nil {
		t.Fatal(err)
	}
	sectorRoot, sectorData, err := createSector()
	if err != nil {
		t.Fatal(err)
	}
	err = smt.sm.AddSector(sectorRoot, 10, sectorData)
	if err != nil {
		t.Fatal(err)
	}
	// Do a probabilistic reset of the storage manager, to verify that the
	// persistence structures can reboot without causing issues.
	err = smt.probabilisticReset()
	if err != nil {
		t.Fatal(err)
	}
	// Check the filesystem - there should be one sector in the storage folder.
	infos, err := ioutil.ReadDir(storageFolderOne)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 {
		t.Fatal("expecting at least one sector in storage folder one")
	}

	// Replace the storage manager dependencies with the faulty remove, and
	// then try to remove the sector.
	smt.sm.dependencies = faultyRemove{}
	err = smt.sm.RemoveSector(sectorRoot, 10)
	if err != mockErrRemoveFile {
		t.Fatal(err)
	}
	// Check that the failed write count was incremented for the storage
	// folder.
	if smt.sm.storageFolders[0].FailedWrites != 1 {
		t.Fatal("failed writes counter is not incrementing properly")
	}
	// Check the filesystem - sector should still be in the storage folder.
	infos, err = ioutil.ReadDir(storageFolderOne)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 {
		t.Fatal("expecting at least one sector in storage folder one")
	}
	// Put 'ffs' back as the set of dependencies.
	smt.sm.dependencies = ffs

	// Add a second storage folder, which can receive the sector when the first
	// storage folder is deleted.
	storageFolderTwo := filepath.Join(smt.persistDir, "driveTwo")
	err = os.Mkdir(storageFolderTwo, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = smt.sm.AddStorageFolder(storageFolderTwo, minimumStorageFolderSize*3)
	if err != nil {
		t.Fatal(err)
	}
	// Do a probabilistic reset of the storage manager, to verify that the
	// persistence structures can reboot without causing issues.
	err = smt.probabilisticReset()
	if err != nil {
		t.Fatal(err)
	}

	// Trigger read errors in storage folder one, which means the storage
	// folder is not going to be able to be deleted successfully.
	ffs.brokenSubstrings = []string{filepath.Join(smt.persistDir, modules.StorageManagerDir, smt.sm.storageFolders[0].uidString())}
	err = smt.sm.RemoveStorageFolder(0, false)
	if err != errIncompleteOffload {
		t.Fatal(err)
	}
	// Check that the storage folder was not removed.
	if len(smt.sm.storageFolders) != 2 {
		t.Fatal("expecting two storage folders after failed remove")
	}
	// Check that the read failure was documented.
	if smt.sm.storageFolders[0].FailedReads != 1 {
		t.Error("expecting a read failure to be reported:", smt.sm.storageFolders[0].FailedReads)
	}
	// Check the filesystem - there should be one sector in the storage folder,
	// and none in storage folder two.
	infos, err = ioutil.ReadDir(storageFolderOne)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 {
		t.Fatal("expecting at least one sector in storage folder one")
	}
	infos, err = ioutil.ReadDir(storageFolderTwo)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 0 {
		t.Fatal("expecting zero sectors in storage folder two")
	}

	// Switch the failure from a read error in the source folder to a write
	// error in the destination folder.
	ffs.brokenSubstrings = []string{filepath.Join(smt.persistDir, modules.StorageManagerDir, smt.sm.storageFolders[1].uidString())}
	err = smt.sm.RemoveStorageFolder(0, false)
	if err != errIncompleteOffload {
		t.Fatal(err)
	}
	// Do a probabilistic reset of the storage manager, to verify that the
	// persistence structures can reboot without causing issues.
	err = smt.probabilisticReset()
	if err != nil {
		t.Fatal(err)
	}
	// Check that the storage folder was not removed.
	if len(smt.sm.storageFolders) != 2 {
		t.Fatal("expecting two storage folders after failed remove")
	}
	// Check that the read failure was documented.
	if smt.sm.storageFolders[1].FailedWrites != 1 {
		t.Error("expecting a read failure to be reported:", smt.sm.storageFolders[1].FailedWrites)
	}
	// Check the filesystem - there should be one sector in the storage folder,
	// and none in storage folder two.
	infos, err = ioutil.ReadDir(storageFolderOne)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 {
		t.Fatal("expecting at least one sector in storage folder one")
	}
	infos, err = ioutil.ReadDir(storageFolderTwo)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 0 {
		t.Fatal("expecting zero sectors in storage folder two")
	}

	// Try to forcibly remove the first storage folder, while in the presence
	// of read errors.
	ffs.brokenSubstrings = []string{filepath.Join(smt.persistDir, modules.StorageManagerDir, smt.sm.storageFolders[0].uidString())}
	uid2 := smt.sm.storageFolders[1].UID
	err = smt.sm.RemoveStorageFolder(0, true)
	if err != nil {
		t.Fatal(err)
	}
	// Do a probabilistic reset of the storage manager, to verify that the
	// persistence structures can reboot without causing issues.
	err = smt.probabilisticReset()
	if err != nil {
		t.Fatal(err)
	}
	// Check that the storage folder was removed.
	if len(smt.sm.storageFolders) != 1 {
		t.Fatal("expecting two storage folders after failed remove")
	}
	if !bytes.Equal(uid2, smt.sm.storageFolders[0].UID) {
		t.Fatal("storage folder was not removed correctly")
	}
	// Check the filesystem - there should be no sectors in storage folder two.
	infos, err = ioutil.ReadDir(storageFolderTwo)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 0 {
		t.Fatal("expecting zero sectors in storage folder two")
	}

	// Add a storage folder with room for sectors. Because storageFolderOne has
	// leftover sectors that the program was unable to clean up (due to disk
	// failure), a third storage folder will be created.
	ffs.brokenSubstrings = nil
	storageFolderThree := filepath.Join(smt.persistDir, "driveThree")
	err = os.Mkdir(storageFolderThree, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = smt.sm.AddStorageFolder(storageFolderThree, minimumStorageFolderSize+modules.SectorSize)
	if err != nil {
		t.Fatal(err)
	}
	// Do a probabilistic reset of the storage manager, to verify that the
	// persistence structures can reboot without causing issues.
	err = smt.probabilisticReset()
	if err != nil {
		t.Fatal(err)
	}

	// Fill up the second storage folder, so that resizes can be attempted with
	// failing disks. storageFolderOne has enough space to store the sectors,
	// but is having disk troubles.
	ffs.brokenSubstrings = []string{filepath.Join(smt.persistDir, modules.StorageManagerDir, smt.sm.storageFolders[1].uidString())}
	numSectors := (minimumStorageFolderSize * 3) / modules.SectorSize
	for i := uint64(0); i < numSectors; i++ {
		sectorRoot, sectorData, err := createSector()
		if err != nil {
			t.Fatal(err)
		}
		err = smt.sm.AddSector(sectorRoot, 11, sectorData)
		if err != nil {
			t.Fatal(err)
		}
		// Do a probabilistic reset of the storage manager, to verify that the
		// persistence structures can reboot without causing issues.
		err = smt.probabilisticReset()
		if err != nil {
			t.Fatal(err)
		}
	}
	// Check the filesystem - storage folder one is having disk issues and
	// should have no sectors. Storage folder two should be full.
	infos, err = ioutil.ReadDir(storageFolderThree)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 0 {
		t.Fatal("expecting zero sectors in storage folder one")
	}
	infos, err = ioutil.ReadDir(storageFolderTwo)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != int(numSectors) {
		t.Fatal("expecting", numSectors, "sectors in storage folder two")
	}
	// Try adding another sector, there should be an error because the one disk
	// is full and the other is having disk troubles.
	sectorRoot, sectorData, err = createSector()
	if err != nil {
		t.Fatal(err)
	}
	err = smt.sm.AddSector(sectorRoot, 11, sectorData)
	if err != errDiskTrouble {
		t.Fatal(err)
	}
	// Do a probabilistic reset of the storage manager, to verify that the
	// persistence structures can reboot without causing issues.
	err = smt.probabilisticReset()
	if err != nil {
		t.Fatal(err)
	}
	// Check the filesystem - storage folder one is having disk issues and
	// should have no sectors. Storage folder two should be full.
	infos, err = ioutil.ReadDir(storageFolderThree)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 0 {
		t.Fatal("expecting zero sectors in storage folder one")
	}
	infos, err = ioutil.ReadDir(storageFolderTwo)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != int(numSectors) {
		t.Fatal("expecting", numSectors, "sectors in storage folder two")
	}

	// Add a third storage folder. Then try to resize the second storage folder
	// such that both storageFolderThree and storageFolderFour have room for
	// the data, but only storageFolderFour is not haivng disk troubles.
	storageFolderFour := filepath.Join(smt.persistDir, "driveFour")
	err = os.Mkdir(storageFolderFour, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = smt.sm.AddStorageFolder(storageFolderFour, minimumStorageFolderSize)
	if err != nil {
		t.Fatal(err)
	}
	// Do a probabilistic reset of the storage manager, to verify that the
	// persistence structures can reboot without causing issues.
	err = smt.probabilisticReset()
	if err != nil {
		t.Fatal(err)
	}
	err = smt.sm.ResizeStorageFolder(0, minimumStorageFolderSize*2)
	if err != nil {
		t.Fatal(err)
	}
	// Do a probabilistic reset of the storage manager, to verify that the
	// persistence structures can reboot without causing issues.
	err = smt.probabilisticReset()
	if err != nil {
		t.Fatal(err)
	}
	// Check the filesystem - storageFolderTwo should have
	// minimumStorageFolderSize*2 worth of sectors, and storageFolderFour
	// should have minimumStorageFolderSize worth of sectors.
	infos, err = ioutil.ReadDir(storageFolderThree)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 0 {
		t.Fatal("expecting zero sectors in storage folder three")
	}
	infos, err = ioutil.ReadDir(storageFolderTwo)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != int(numSectors)-int(minimumStorageFolderSize/modules.SectorSize) {
		t.Fatal("expecting", numSectors, "sectors in storage folder two")
	}
	infos, err = ioutil.ReadDir(storageFolderFour)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != int(minimumStorageFolderSize/modules.SectorSize) {
		t.Fatal("expecting to have 8 sectors in storageFolderFour")
	}

	// Trigger an incomplete disk transfer by adding room for one more sector
	// to storageFolderFour, but then trying to remove a bunch of sectors from
	// storageFolderTwo. There is enough room on storage folder 3 to make the
	// operation successful, but it is having disk troubles.
	err = smt.sm.ResizeStorageFolder(2, minimumStorageFolderSize+modules.SectorSize)
	if err != nil {
		t.Fatal(err)
	}
	err = smt.sm.ResizeStorageFolder(0, minimumStorageFolderSize)
	if err != errIncompleteOffload {
		t.Fatal(err)
	}
	// Do a probabilistic reset of the storage manager, to verify that the
	// persistence structures can reboot without causing issues.
	err = smt.probabilisticReset()
	if err != nil {
		t.Fatal(err)
	}
	// Check that the sizes of the storage folders have been updated correctly.
	if smt.sm.storageFolders[0].Size != minimumStorageFolderSize*2-modules.SectorSize {
		t.Error("storage folder size was not decreased correctly during the shrink operation")
	}
	if smt.sm.storageFolders[0].SizeRemaining != 0 {
		t.Error("storage folder size remaining was not updated correctly after failed shrink operation")
	}
	// Check the filesystem - there should be one less sector in
	// storageFolderTwo from the previous check, and one more sector in
	// storageFolderFour.
	infos, err = ioutil.ReadDir(storageFolderThree)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 0 {
		t.Fatal("expecting zero sectors in storage folder three")
	}
	infos, err = ioutil.ReadDir(storageFolderTwo)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != int(numSectors)-int(minimumStorageFolderSize/modules.SectorSize)-1 {
		t.Fatal("expecting", numSectors, "sectors in storage folder two")
	}
	infos, err = ioutil.ReadDir(storageFolderFour)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != int(minimumStorageFolderSize/modules.SectorSize)+1 {
		t.Fatal("filesystem consistency error")
	}
}
*/
