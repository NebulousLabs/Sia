package renter

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/fastrand"
)

// newTestingFile initializes a file object with random parameters.
func newTestingFile() *file {
	data := fastrand.Bytes(8)
	nData := fastrand.Intn(10)
	nParity := fastrand.Intn(10)
	rsc, _ := NewRSCode(nData+1, nParity+1)

	return &file{
		name:        "testfile-" + strconv.Itoa(int(data[0])),
		size:        encoding.DecUint64(data[1:5]),
		masterKey:   crypto.GenerateTwofishKey(),
		erasureCode: rsc,
		pieceSize:   encoding.DecUint64(data[6:8]),
		staticUID:   persist.RandomSuffix(),
	}
}

// equalFiles is a helper function that compares two files for equality.
func equalFiles(f1, f2 *file) error {
	if f1 == nil || f2 == nil {
		return fmt.Errorf("one or both files are nil")
	}
	if f1.name != f2.name {
		return fmt.Errorf("names do not match: %v %v", f1.name, f2.name)
	}
	if f1.size != f2.size {
		return fmt.Errorf("sizes do not match: %v %v", f1.size, f2.size)
	}
	if f1.masterKey != f2.masterKey {
		return fmt.Errorf("keys do not match: %v %v", f1.masterKey, f2.masterKey)
	}
	if f1.pieceSize != f2.pieceSize {
		return fmt.Errorf("pieceSizes do not match: %v %v", f1.pieceSize, f2.pieceSize)
	}
	return nil
}

// TestFileMarshalling tests the MarshalSia and UnmarshalSia functions of the
// file type.
func TestFileMarshalling(t *testing.T) {
	savedFile := newTestingFile()
	buf := new(bytes.Buffer)
	savedFile.MarshalSia(buf)

	loadedFile := new(file)
	err := loadedFile.UnmarshalSia(buf)
	if err != nil {
		t.Fatal(err)
	}

	err = equalFiles(savedFile, loadedFile)
	if err != nil {
		t.Fatal(err)
	}
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
	rt.renter.files[savedFile.name] = savedFile
	rt.renter.mu.Unlock(id)

	// Share .sia file to disk.
	path := filepath.Join(build.SiaTestingDir, "renter", t.Name(), "test.sia")
	err = rt.renter.ShareFiles([]string{savedFile.name}, path)
	if err != nil {
		t.Fatal(err)
	}

	// Remove the file from the renter.
	delete(rt.renter.files, savedFile.name)

	// Load the .sia file back into the renter.
	names, err := rt.renter.LoadSharedFiles(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != savedFile.name {
		t.Fatal("nickname not loaded properly:", names)
	}
	err = equalFiles(rt.renter.files[savedFile.name], savedFile)
	if err != nil {
		t.Fatal(err)
	}

	// Share and load multiple files.
	savedFile2 := newTestingFile()
	rt.renter.files[savedFile2.name] = savedFile2
	path = filepath.Join(build.SiaTestingDir, "renter", t.Name(), "test2.sia")
	err = rt.renter.ShareFiles([]string{savedFile.name, savedFile2.name}, path)
	if err != nil {
		t.Fatal(err)
	}

	// Remove the files from the renter.
	delete(rt.renter.files, savedFile.name)
	delete(rt.renter.files, savedFile2.name)

	names, err = rt.renter.LoadSharedFiles(path)
	if err != nil {
		t.Fatal(nil)
	}
	if len(names) != 2 || (names[0] != savedFile2.name && names[1] != savedFile2.name) {
		t.Fatal("nicknames not loaded properly:", names)
	}
	err = equalFiles(rt.renter.files[savedFile.name], savedFile)
	if err != nil {
		t.Fatal(err)
	}
	err = equalFiles(rt.renter.files[savedFile2.name], savedFile2)
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
	rt.renter.files[savedFile.name] = savedFile
	rt.renter.mu.Unlock(id)

	ascii, err := rt.renter.ShareFilesASCII([]string{savedFile.name})
	if err != nil {
		t.Fatal(err)
	}

	// Remove the file from the renter.
	delete(rt.renter.files, savedFile.name)

	names, err := rt.renter.LoadSharedFilesASCII(ascii)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != savedFile.name {
		t.Fatal("nickname not loaded properly")
	}

	err = equalFiles(rt.renter.files[savedFile.name], savedFile)
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

	// Create and save some files
	var f1, f2, f3 *file
	f1 = newTestingFile()
	f2 = newTestingFile()
	f3 = newTestingFile()
	// names must not conflict
	for f2.name == f1.name || f2.name == f3.name {
		f2 = newTestingFile()
	}
	for f3.name == f1.name || f3.name == f2.name {
		f3 = newTestingFile()
	}
	rt.renter.saveFile(f1)
	rt.renter.saveFile(f2)
	rt.renter.saveFile(f3)

	err = rt.renter.saveSync() // save metadata
	if err != nil {
		t.Fatal(err)
	}

	// load should now load the files into memory.
	id := rt.renter.mu.Lock()
	err = rt.renter.load()
	rt.renter.mu.Unlock(id)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	if err := equalFiles(f1, rt.renter.files[f1.name]); err != nil {
		t.Fatal(err)
	}
	if err := equalFiles(f2, rt.renter.files[f2.name]); err != nil {
		t.Fatal(err)
	}
	if err := equalFiles(f3, rt.renter.files[f3.name]); err != nil {
		t.Fatal(err)
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
	f1.name = "foo"
	f2 := newTestingFile()
	f2.name = "foo/bar"
	f3 := newTestingFile()
	f3.name = "foo/bar/baz"
	rt.renter.saveFile(f1)
	rt.renter.saveFile(f2)
	rt.renter.saveFile(f3)

	// Load the files into the renter.
	id := rt.renter.mu.Lock()
	err = rt.renter.load()
	rt.renter.mu.Unlock(id)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	// Check that the files were loaded properly.
	if err := equalFiles(f1, rt.renter.files[f1.name]); err != nil {
		t.Fatal(err)
	}
	if err := equalFiles(f2, rt.renter.files[f2.name]); err != nil {
		t.Fatal(err)
	}
	if err := equalFiles(f3, rt.renter.files[f3.name]); err != nil {
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
	expWalkStr := (f3.name + ".sia") + (f2.name + ".sia") + (f1.name + ".sia")
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

// TestUpgradeLegacyPersistFile
func TestUpgradeLegacyPersistFile(t *testing.T) {
	// Create renter
	r := &Renter{}
	r.persistDir = filepath.Join("..", "..", "persist", "testdata")

	// Set persist version to legacy version and save persist file
	settingsMetadata.Version = persistVersion040
	data := struct {
		Tracking  map[string]string
		Repairing map[string]string
	}{}
	persist.SaveJSON(settingsMetadata, data, filepath.Join(r.persistDir, PersistFilename))

	// Confirm loading of legacy persist file
	r.persist = persistence{
		Tracking: make(map[string]trackedFile),
	}
	settingsMetadata.Version = persistVersion
	err := r.load()
	if err != nil {
		t.Fatal(err)
	}
}
