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

// TestSafeFile tests creating and committing safe files with both relative and
// absolute paths.
func TestSafeFile(t *testing.T) {
	// Generate absolute path filename.
	absPath := filepath.Join(os.TempDir(), "NewSafeFile test file"+RandomSuffix())
	// Get relative path filename from absolute path.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relPath, err := filepath.Rel(wd, absPath)
	if err != nil {
		t.Fatal(err)
	}

	// Test creating and committing a safe file with each filename.
	filenames := []string{absPath, relPath}
	for _, filename := range filenames {
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
			t.Errorf("safeFile created with filename: %s has temporary filename that is equivalent to finalName: %s\n", filename, sf.Name())
		}

		// Check that committing doesn't return an error.
		sf.Close()
		err = sf.Commit()
		if err != nil {
			t.Fatal(err)
		}

		// Check that commiting moved the file to the originally specified path.
		_, err = os.Stat(filename)
		if err != nil {
			t.Fatalf("safeFile created with filename: %s not committed correctly to: %s\n", filename, sf.finalName)
		}
	}
}
