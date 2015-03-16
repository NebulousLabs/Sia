package tester

import (
	"os"
	"path/filepath"
)

const (
	SiaTestingDir = "SiaTesting"
)

// TempDir returns the tempified version of the input directory, after creating
// the tempified directory.
func TempDir(dir string) string {
	temp := os.TempDir()
	tempified := filepath.Join(temp, SiaTestingDir, dir)
	return tempified
}
