package siatest

import (
	"os"
	"path/filepath"
)

var (
	// SiaTestingDir is the directory that contains all of the files and
	// folders created during testing.
	SiaTestingDir = filepath.Join(os.TempDir(), "SiaTesting")
)

// TestDir joins the provided directories and prefixes them with the Sia
// testing directory, removing any files or directories that previously existed
// at that location.
func TestDir(dirs ...string) (string, error) {
	path := filepath.Join(SiaTestingDir, filepath.Join(dirs...))
	err := os.RemoveAll(path)
	if err != nil {
		return "", err
	}
	return path, nil
}
