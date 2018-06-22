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

// DataDir returns a temporary directory that will be used to create temporary
// files for uploading.
func DataDir() string {
	path := filepath.Join(SiaTestingDir, "siatest", "data")
	if err := os.MkdirAll(path, 0777); err != nil {
		panic(err)
	}
	return path
}
