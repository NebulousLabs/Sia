package renter

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/siafile"

	"github.com/NebulousLabs/fastrand"
)

// newTestingFile initializes a file object with random parameters.
func newTestingFile() *siafile.SiaFile {
	data := fastrand.Bytes(8)
	nData := fastrand.Intn(10)
	nParity := fastrand.Intn(10)
	rsc, _ := NewRSCode(nData+1, nParity+1)

	name := "testfile-" + strconv.Itoa(int(data[0]))

	return siafile.New(name, rsc, pieceSize, 1000)
}

// equalFiles is a helper function that compares two files for equality.
func equalFiles(f1, f2 *siafile.SiaFile) error {
	if f1 == nil || f2 == nil {
		return fmt.Errorf("one or both files are nil")
	}
	if f1.SiaPath() != f2.SiaPath() {
		return fmt.Errorf("names do not match: %v %v", f1.SiaPath(), f2.SiaPath())
	}
	if f1.Size() != f2.Size() {
		return fmt.Errorf("sizes do not match: %v %v", f1.Size(), f2.Size())
	}
	if f1.MasterKey() != f2.MasterKey() {
		return fmt.Errorf("keys do not match: %v %v", f1.MasterKey(), f2.MasterKey())
	}
	if f1.PieceSize() != f2.PieceSize() {
		return fmt.Errorf("pieceSizes do not match: %v %v", f1.PieceSize(), f2.PieceSize())
	}
	return nil
}

// TestFileShareLoad tests the sharing/loading functions of the renter.
func TestFileShareLoad(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rt, err := newRenterTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Create a file and add it to the renter.
	savedFile := newTestingFile()
	id := rt.renter.mu.Lock()
	rt.renter.files[savedFile.SiaPath()] = savedFile
	rt.renter.mu.Unlock(id)

	// Share .sia file to disk.
	path := filepath.Join(build.SiaTestingDir, "renter", t.Name(), "test.sia")
	err = rt.renter.ShareFiles([]string{savedFile.SiaPath()}, path)
	if err != nil {
		t.Fatal(err)
	}

	// Remove the file from the renter.
	delete(rt.renter.files, savedFile.SiaPath())

	// Load the .sia file back into the renter.
	names, err := rt.renter.LoadSharedFiles(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != savedFile.SiaPath() {
		t.Fatal("nickname not loaded properly:", names)
	}
	err = equalFiles(rt.renter.files[savedFile.SiaPath()], savedFile)
	if err != nil {
		t.Fatal(err)
	}

	// Share and load multiple files.
	savedFile2 := newTestingFile()
	rt.renter.files[savedFile2.SiaPath()] = savedFile2
	path = filepath.Join(build.SiaTestingDir, "renter", t.Name(), "test2.sia")
	err = rt.renter.ShareFiles([]string{savedFile.SiaPath(), savedFile2.SiaPath()}, path)
	if err != nil {
		t.Fatal(err)
	}

	// Remove the files from the renter.
	delete(rt.renter.files, savedFile.SiaPath())
	delete(rt.renter.files, savedFile2.SiaPath())

	names, err = rt.renter.LoadSharedFiles(path)
	if err != nil {
		t.Fatal(nil)
	}
	if len(names) != 2 || (names[0] != savedFile2.SiaPath() && names[1] != savedFile2.SiaPath()) {
		t.Fatal("nicknames not loaded properly:", names)
	}
	err = equalFiles(rt.renter.files[savedFile.SiaPath()], savedFile)
	if err != nil {
		t.Fatal(err)
	}
	err = equalFiles(rt.renter.files[savedFile2.SiaPath()], savedFile2)
	if err != nil {
		t.Fatal(err)
	}
}

// TestFileShareLoadASCII tests the ASCII sharing/loading functions.
func TestFileShareLoadASCII(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rt, err := newRenterTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Create a file and add it to the renter.
	savedFile := newTestingFile()
	id := rt.renter.mu.Lock()
	rt.renter.files[savedFile.SiaPath()] = savedFile
	rt.renter.mu.Unlock(id)

	ascii, err := rt.renter.ShareFilesASCII([]string{savedFile.SiaPath()})
	if err != nil {
		t.Fatal(err)
	}

	// Remove the file from the renter.
	delete(rt.renter.files, savedFile.SiaPath())

	names, err := rt.renter.LoadSharedFilesASCII(ascii)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != savedFile.SiaPath() {
		t.Fatal("nickname not loaded properly")
	}

	err = equalFiles(rt.renter.files[savedFile.SiaPath()], savedFile)
	if err != nil {
		t.Fatal(err)
	}
}

// TestRenterSaveLoad probes the save and load methods of the renter type.
func TestRenterSaveLoad(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rt, err := newRenterTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Check that the default values got set correctly.
	settings := rt.renter.Settings()
	if settings.MaxDownloadSpeed != DefaultMaxDownloadSpeed {
		t.Error("default max download speed not set at init")
	}
	if settings.MaxUploadSpeed != DefaultMaxUploadSpeed {
		t.Error("default max upload speed not set at init")
	}
	if settings.StreamCacheSize != DefaultStreamCacheSize {
		t.Error("default stream cache size not set at init")
	}

	// Create and save some files
	var f1, f2, f3 *siafile.SiaFile
	f1 = newTestingFile()
	f2 = newTestingFile()
	f3 = newTestingFile()
	// names must not conflict
	for f2.SiaPath() == f1.SiaPath() || f2.SiaPath() == f3.SiaPath() {
		f2 = newTestingFile()
	}
	for f3.SiaPath() == f1.SiaPath() || f3.SiaPath() == f2.SiaPath() {
		f3 = newTestingFile()
	}
	rt.renter.saveFile(f1)
	rt.renter.saveFile(f2)
	rt.renter.saveFile(f3)

	// Update the settings of the renter to have a new stream cache size and
	// download speed.
	newDownSpeed := int64(300e3)
	newUpSpeed := int64(500e3)
	newCacheSize := uint64(3)
	settings.MaxDownloadSpeed = newDownSpeed
	settings.MaxUploadSpeed = newUpSpeed
	settings.StreamCacheSize = newCacheSize
	rt.renter.SetSettings(settings)

	err = rt.renter.saveSync() // save metadata
	if err != nil {
		t.Fatal(err)
	}
	err = rt.renter.Close()
	if err != nil {
		t.Fatal(err)
	}

	// load should now load the files into memory.
	rt.renter, err = New(rt.gateway, rt.cs, rt.wallet, rt.tpool, filepath.Join(rt.dir, modules.RenterDir))
	if err != nil {
		t.Fatal(err)
	}

	if err := equalFiles(f1, rt.renter.files[f1.SiaPath()]); err != nil {
		t.Fatal(err)
	}
	if err := equalFiles(f2, rt.renter.files[f2.SiaPath()]); err != nil {
		t.Fatal(err)
	}
	if err := equalFiles(f3, rt.renter.files[f3.SiaPath()]); err != nil {
		t.Fatal(err)
	}

	newSettings := rt.renter.Settings()
	if newSettings.MaxDownloadSpeed != newDownSpeed {
		t.Error("download settings not being persisted correctly")
	}
	if newSettings.MaxUploadSpeed != newUpSpeed {
		t.Error("upload settings not being persisted correctly")
	}
	if newSettings.StreamCacheSize != newCacheSize {
		t.Error("cache settings not being persisted correctly")
	}
}

// TestRenterPaths checks that the renter properly handles nicknames
// containing the path separator ("/").
func TestRenterPaths(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rt, err := newRenterTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Create and save some files.
	// The result of saving these files should be a directory containing:
	//   foo.sia
	//   foo/bar.sia
	//   foo/bar/baz.sia
	f1 := newTestingFile()
	f1.Rename("foo")
	f2 := newTestingFile()
	f2.Rename("foo/bar")
	f3 := newTestingFile()
	f3.Rename("foo/bar/baz")
	rt.renter.saveFile(f1)
	rt.renter.saveFile(f2)
	rt.renter.saveFile(f3)

	// Restart the renter to re-do the init cycle.
	err = rt.renter.Close()
	if err != nil {
		t.Fatal(err)
	}
	rt.renter, err = New(rt.gateway, rt.cs, rt.wallet, rt.tpool, filepath.Join(rt.dir, modules.RenterDir))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the files were loaded properly.
	if err := equalFiles(f1, rt.renter.files[f1.SiaPath()]); err != nil {
		t.Fatal(err)
	}
	if err := equalFiles(f2, rt.renter.files[f2.SiaPath()]); err != nil {
		t.Fatal(err)
	}
	if err := equalFiles(f3, rt.renter.files[f3.SiaPath()]); err != nil {
		t.Fatal(err)
	}

	// To confirm that the file structure was preserved, we walk the renter
	// folder and emit the name of each .sia file encountered (filepath.Walk
	// is deterministic; it orders the files lexically).
	var walkStr string
	filepath.Walk(rt.renter.persistDir, func(path string, _ os.FileInfo, _ error) error {
		// capture only .sia files
		if filepath.Ext(path) != ".sia" {
			return nil
		}
		rel, _ := filepath.Rel(rt.renter.persistDir, path) // strip testdir prefix
		walkStr += rel
		return nil
	})
	// walk will descend into foo/bar/, reading baz, bar, and finally foo
	expWalkStr := (f3.SiaPath() + ".sia") + (f2.SiaPath() + ".sia") + (f1.SiaPath() + ".sia")
	if filepath.ToSlash(walkStr) != expWalkStr {
		t.Fatalf("Bad walk string: expected %v, got %v", expWalkStr, walkStr)
	}
}

// TestSiafileCompatibility tests that the renter is able to load v0.4.8 .sia files.
func TestSiafileCompatibility(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rt, err := newRenterTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Load the compatibility file into the renter.
	path := filepath.Join("..", "..", "compatibility", "siafile_v0.4.8.sia")
	names, err := rt.renter.LoadSharedFiles(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "testfile-183" {
		t.Fatal("nickname not loaded properly:", names)
	}
}
