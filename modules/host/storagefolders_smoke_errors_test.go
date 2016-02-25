package host

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
	return ioutil.ReadFile(s)
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
	return os.Symlink(s1, s2)
}

// writeFile reads a file from the filesystem. The call will fail if reading
// from a file that has a substring which matches the ffs list of broken
// substrings.
func (ffs faultyFS) writeFile(s string, b []byte, fm os.FileMode) error {
	for _, bs := range ffs.brokenSubstrings {
		if strings.Contains(s, bs) {
			return mockErrWriteFile
		}
	}
	return ioutil.WriteFile(s, b, fm)
}

// faultyRemove is a mocked set of dependencies that operates as normal except
// that removeFile will fail.
type faultyRemove struct{}

// removeFile fails to remove a file from the filesystem.
func (faultyRemove) removeFile(s string) error {
	return mockErrRemoveFile
}

// TestStorageFolderTolerance tests the tolerance of storage folders in the
// presense of disk failures. Disk failures should be recorded, and the
// failures should be handled gracefully - nonfailing disks should not have
// problems.
func TestStorageFolderTolerance(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := blankHostTester("TestStorageFolderTolerance")
	if err != nil {
		t.Fatal(err)
	}
	// Replace the host so that it's using faultyOS for its dependencies.
	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	ffs := new(faultyFS)
	ht.host, err = newHost(ffs, ht.cs, ht.tpool, ht.wallet, ":0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}

	// Add a storage folder when the symlinking is failing.
	storageFolderOne := filepath.Join(ht.persistDir, "driveOne")
	ffs.brokenSubstrings = []string{storageFolderOne}
	err = os.Mkdir(storageFolderOne, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.AddStorageFolder(storageFolderOne, minimumStorageFolderSize)
	if err != mockErrSymlink {
		t.Fatal(err)
	}

	// Add storage folder one without errors, and then add a sector to the
	// storage folder.
	ffs.brokenSubstrings = nil
	err = ht.host.AddStorageFolder(storageFolderOne, minimumStorageFolderSize)
	if err != nil {
		t.Fatal(err)
	}
	sectorRoot, sectorData, err := createSector()
	if err != nil {
		t.Fatal(err)
	}
	ht.host.mu.Lock()
	err = ht.host.addSector(sectorRoot, 10, sectorData)
	ht.host.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	// Add a second storage folder, which can receive the sector when the first
	// storage folder is deleted.
	storageFolderTwo := filepath.Join(ht.persistDir, "driveTwo")
	err = os.Mkdir(storageFolderTwo, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.AddStorageFolder(storageFolderTwo, minimumStorageFolderSize)
	if err != nil {
		t.Fatal(err)
	}

	// Trigger read errors in storage folder one, which means the storage
	// folder is not going to be able to be deleted successfully.
	ffs.brokenSubstrings = []string{filepath.Join(ht.persistDir, modules.HostDir, ht.host.storageFolders[0].uidString())}
	err = ht.host.RemoveStorageFolder(0, false)
	if err != errIncompleteOffload {
		t.Fatal(err)
	}
	// Check that the storage folder was not removed.
	if len(ht.host.storageFolders) != 2 {
		t.Fatal("expecting two storage folders after failed remove")
	}
	// Check that the read failure was documented.
	if ht.host.storageFolders[0].FailedReads != 1 {
		t.Error("expecting a read failure to be reported:", ht.host.storageFolders[0].FailedReads)
	}

	// Switch the failure from a read error in the source folder to a write
	// error in the destination folder.
	ffs.brokenSubstrings = []string{filepath.Join(ht.persistDir, modules.HostDir, ht.host.storageFolders[1].uidString())}
	err = ht.host.RemoveStorageFolder(0, false)
	if err != errIncompleteOffload {
		t.Fatal(err)
	}
	// Check that the storage folder was not removed.
	if len(ht.host.storageFolders) != 2 {
		t.Fatal("expecting two storage folders after failed remove")
	}
	// Check that the read failure was documented.
	if ht.host.storageFolders[1].FailedWrites != 1 {
		t.Error("expecting a read failure to be reported:", ht.host.storageFolders[1].FailedWrites)
	}

	// Try to forcibly remove the first storage folder, while in the presence
	// of read errors.
	ffs.brokenSubstrings = []string{filepath.Join(ht.persistDir, modules.HostDir, ht.host.storageFolders[0].uidString())}
	uid2 := ht.host.storageFolders[1].UID
	err = ht.host.RemoveStorageFolder(0, true)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the storage folder was removed.
	if len(ht.host.storageFolders) != 1 {
		t.Fatal("expecting two storage folders after failed remove")
	}
	if !bytes.Equal(uid2, ht.host.storageFolders[0].UID) {
		t.Fatal("storage folder was not removed correctly")
	}

	// TODO: resize a storage folder where the filesystem fails for the first
	// read and not any of the others.

	// TODO: add a sector when the emptiest drive is having write failures.
}
