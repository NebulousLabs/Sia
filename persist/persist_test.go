package persist

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"gitlab.com/NebulousLabs/Sia/build"
	"gitlab.com/NebulousLabs/fastrand"
)

// TestIntegrationRandomSuffix checks that the random suffix creator creates
// valid files.
func TestIntegrationRandomSuffix(t *testing.T) {
	tmpDir := build.TempDir(persistDir, t.Name())
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
	tmpDir := build.TempDir(persistDir, t.Name())
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
	data := fastrand.Bytes(10)
	_, err = sf.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	err = sf.CommitSync()
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
// relative paths. Specifically, we test that calling os.Chdir between creating
// and committing a safe file doesn't affect the safe file's final path. The
// relative path tested is relative to the working directory.
func TestRelativePathSafeFile(t *testing.T) {
	tmpDir := build.TempDir(persistDir, t.Name())
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
	if err != nil {
		t.Fatal(err)
	}

	// Create safe file.
	sf, err := NewSafeFile(relPath)
	if err != nil {
		t.Fatal(err)
	}
	defer sf.Close()

	// Check that the path of the file is not equal to the final path of the
	// file.
	if sf.Name() == absPath {
		t.Errorf("safeFile created with filename: %s has temporary filename that is equivalent to finalName: %s\n", absPath, sf.Name())
	}

	// Write random data to the file.
	data := fastrand.Bytes(10)
	_, err = sf.Write(data)
	if err != nil {
		t.Fatal(err)
	}

	// Change directories and commit.
	tmpChdir := build.TempDir(persistDir, t.Name()+"2")
	err = os.MkdirAll(tmpChdir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	os.Chdir(tmpChdir)
	defer os.Chdir(wd)
	err = sf.CommitSync()
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
