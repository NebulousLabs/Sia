package persist

import (
	"bytes"
	"crypto/rand"
	"io/ioutil"
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

// TestAbsolutePathSafeFile tests creating and committing safe files with
// absolute paths.
func TestAbsolutePathSafeFile(t *testing.T) {
	tmpDir := build.TempDir(persistDir, "TestAbsolutePathSafeFile")
	err := os.MkdirAll(tmpDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	absPath := filepath.Join(tmpDir, "test")

	// Create safe file.
	sf, err := NewSafeFile(absPath)
	defer sf.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Check that the name of the file is not equal to the final name of the
	// file.
	if sf.Name() == absPath {
		t.Errorf("safeFile created with filename: %s has temporary filename that is equivalent to finalName: %s\n", absPath, sf.Name())
	}

	// Write random data to the file and commit.
	data := make([]byte, 10)
	rand.Read(data)
	_, err = sf.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	err = sf.Commit()
	if err != nil {
		t.Fatal(err)
	}

	// Check that the file exists and has same data that was written to it.
	dataRead, err := ioutil.ReadFile(absPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, dataRead) {
		t.Fatalf("Committed file has different data than was written to it: expected %v, got %v\n", data, dataRead)
	}
}

// TestRelativePathSafeFile tests creating and committing safe files with
// relative paths. Relative paths are testing to test that calling os.Chdir
// inbetween creating and committing a safe file doesn't affect the safe file's
// final path. The relative path tested is relative to the working directory.
func TestRelativePathSafeFile(t *testing.T) {
	tmpDir := build.TempDir(persistDir, "TestRelativePathSafeFile")
	err := os.MkdirAll(tmpDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	absPath := filepath.Join(tmpDir, "test")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relPath, err := filepath.Rel(wd, absPath)

	// Create safe file.
	sf, err := NewSafeFile(relPath)
	defer sf.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Check that the path of the file is not equal to the final path of the
	// file.
	if sf.Name() == absPath {
		t.Errorf("safeFile created with filename: %s has temporary filename that is equivalent to finalName: %s\n", absPath, sf.Name())
	}

	// Write random data to the file.
	data := make([]byte, 10)
	rand.Read(data)
	_, err = sf.Write(data)
	if err != nil {
		t.Fatal(err)
	}

	// Change directories and commit.
	tmpChdir := build.TempDir(persistDir, "TestRelativePathSafeFileTmpChdir")
	err = os.MkdirAll(tmpChdir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	os.Chdir(tmpChdir)
	defer os.Chdir(wd)
	err = sf.Commit()
	if err != nil {
		t.Fatal(err)
	}

	// Check that the file exists and has same data that was written to it.
	dataRead, err := ioutil.ReadFile(absPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, dataRead) {
		t.Fatalf("Committed file has different data than was written to it: expected %v, got %v\n", data, dataRead)
	}
}
