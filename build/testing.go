package build

import (
	"os"
	"path/filepath"
)

var (
	SiaTestingDir = filepath.Join(os.TempDir(), "SiaTesting")
)

// TempDir joins the provided directories and prefixes them with the Sia
// testing directory.
func TempDir(dirs ...string) string {
	path := filepath.Join(SiaTestingDir, filepath.Join(dirs...))
	os.RemoveAll(path) // remove old test data
	return path
}
