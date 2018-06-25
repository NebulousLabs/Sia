package siatest

import (
	"os"
	"path/filepath"
	"testing"
)

var (
	// SiaTestingDir is the directory that contains all of the files and
	// folders created during testing.
	SiaTestingDir = filepath.Join(os.TempDir(), "SiaTesting")
)

// TestDir joins the provided directories and prefixes them with the Sia
// testing directory, removing any files or directories that previously existed
// at that location.
func TestDir(dirs ...string) string {
	path := filepath.Join(SiaTestingDir, "siatest", filepath.Join(dirs...))
	err := os.RemoveAll(path)
	if err != nil {
		panic(err)
	}
	return path
}

// siatestTestDir creates a testing directory for tests within the siatest
// module.
func siatestTestDir(t *testing.T) string {
	path := TestDir("siatest", t.Name())
	if err := os.MkdirAll(path, 0777); err != nil {
		panic(err)
	}
	return path
}

// filesDir returns the path to the files directory of the TestNode. The files
// directory is where new files are stored before being uploaded.
func (tn *TestNode) filesDir() string {
	path := filepath.Join(tn.Dir, "files")
	if err := os.MkdirAll(path, 0777); err != nil {
		panic(err)
	}
	return path
}

// downloadsDir returns the path to the download directory of the TestNode.
func (tn *TestNode) downloadsDir() string {
	path := filepath.Join(tn.Dir, "downloads")
	if err := os.MkdirAll(path, 0777); err != nil {
		panic(err)
	}
	return path
}
