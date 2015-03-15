package tester

import (
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/sync"
)

const (
	SiaTestingDir = "SiaTesting"
)

var (
	// The ports used during testing start at 12e3 and increment as new ports
	// are requested. This is to prevent port collisions during testing.
	availablePort      int = 12e3
	availablePortMutex sync.RWMutex
)

// NewPort returns a new, unique port number.
func NewPort() int {
	id := availablePortMutex.Lock()
	port := availablePort
	availablePort++
	availablePortMutex.Unlock(id)
	return port
}

// TempDir returns the tempified version of the input directory, after creating
// the tempified directory.
func TempDir(dir string) string {
	temp := os.TempDir()
	tempified := filepath.Join(temp, SiaTestingDir, dir)
	return tempified
}
