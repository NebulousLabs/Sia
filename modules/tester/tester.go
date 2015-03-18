package tester

import (
	"os"
	"path/filepath"
)

const (
	SiaTestingDir = "SiaTesting"
)

// TempDir takes a set of directory names and joins them to the sia temp
// folder.
func TempDir(dirs ...string) string {
	tmp := append([]string{os.TempDir()}, SiaTestingDir)
	full := append(tmp, dirs...)
	return filepath.Join(full...) // filepath.Join(tmp, testing, dirs...) returns 'too many arguments' error.
}
