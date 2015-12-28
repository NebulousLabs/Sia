package persist

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
)

// TestIntegrationRandomSuffix checks that the random suffix creator creates
// valid files.
func TestIntegrationRandomSuffix(t *testing.T) {
	tmpDir := build.TempDir(persistDir, "TestIntegrationRandomSuffix")
	err := os.MkdirAll(tmpDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		suffix := RandomSuffix()
		filename := filepath.Join(tmpDir, "test file - "+suffix+".nil")
		file, err := os.Create(filename)
		if err != nil {
			t.Fatal(err)
		}
		file.Close()
	}
}

// TestUnitNewSafeFile checks that a new file is created and that its name is
// different than the finalName of the new safeFile.
func TestUnitNewSafeFile(t *testing.T) {
	// Create safe file.
	sf, err := NewSafeFile("NewSafeFile test file" + RandomSuffix())
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(sf.Name())
	defer sf.Close()

	// Check that the name of the file is not equal to the final name of the
	// file.
	if sf.Name() == sf.finalName {
		t.Errorf("safeFile temporary filename and finalName are equivalent: %s\n", sf.Name())
	}
}

// testSafeFileWithPath tests creating and committing safe files with a
// specified path.
func testSafeFileWithPath(filename string, t *testing.T) {
	// Create safe file.
	sf, err := NewSafeFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	// These two defers seem redundant but are not. If sf.Commit() fails to
	// move the file then the first defer is necessary. Otherwise the second
	// defer is necessary.
	defer os.Remove(sf.Name())
	defer os.Remove(sf.finalName)

	// Check that the name of the file is not equal to the final name of the
	// file.
	if sf.Name() == sf.finalName {
		t.Errorf("safeFile temporary filename and finalName are equivalent: %s\n", sf.Name())
	}

	sf.Close()
	err = sf.Commit()
	if err != nil {
		t.Fatal(err)
	}

	// Check that commiting moved the file to the originally specified path.
	_, err = os.Stat(filename)
	if err != nil {
		t.Fatal("safeFile not committed correctly.")
	}
}

// TestAbsolutePathSafeFile tests that safe files created with an absolute path
// are created and committed correctly.
func TestAbsolutePathSafeFile(t *testing.T) {
	filename := filepath.Join(os.TempDir(), "NewSafeFile test file"+RandomSuffix())
	absFilename, _ := filepath.Abs(filename)
	testSafeFileWithPath(absFilename, t)
}

// TestRelativePathSafeFile tests that safe files created with a relative path
// are created and committed correctly.
func TestRelativePathSafeFile(t *testing.T) {
	filename := filepath.Join(os.TempDir(), "NewSafeFile test file"+RandomSuffix())
	testSafeFileWithPath(filename, t)
}
