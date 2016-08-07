package contractmanager

import (
	"crypto/rand"
	"errors"
	"io"
	"os"
	"time"

	"github.com/NebulousLabs/Sia/persist"
)

// Fake errors that get returned when a simulated failure of a dependency is
// desired for testing.
var (
	mockErrListen       = errors.New("simulated Listen failure")
	mockErrLoadFile     = errors.New("simulated LoadFile failure")
	mockErrMkdirAll     = errors.New("simulated MkdirAll failure")
	mockErrNewLogger    = errors.New("simulated NewLogger failure")
	mockErrOpenDatabase = errors.New("simulated OpenDatabase failure")
	mockErrReadFile     = errors.New("simulated ReadFile failure")
	mockErrRemoveFile   = errors.New("simulated RemoveFile faulure")
	mockErrSymlink      = errors.New("simulated Symlink failure")
	mockErrWriteFile    = errors.New("simulated WriteFile failure")
)

// These interfaces define the StorageManager's dependencies. Mocking
// implementation complexity can be reduced by defining each dependency as the
// minimum possible subset of the real dependency.
type (
	// dependencies defines all of the dependencies of the StorageManager.
	dependencies interface {
		// afterDuration gives the caller the ability to wait for a certain
		// duration before receiving on a channel.
		afterDuration(time.Duration) <-chan time.Time

		// createFile gives the host the ability to create files on the
		// operating system.
		createFile(string) (file, error)

		// loadFile allows the host to load a persistence structure form disk.
		loadFile(persist.Metadata, interface{}, string) error

		// mkdirAll gives the host the ability to create chains of folders
		// within the filesystem.
		mkdirAll(string, os.FileMode) error

		// newLogger creates a logger that the host can use to log messages and
		// write critical statements.
		newLogger(string) (*persist.Logger, error)

		// randRead fills the input bytes with random data.
		randRead([]byte) (int, error)
	}

	// file implements all of the methods that can be called on an os.File.
	file interface {
		io.Closer
		io.Writer
		Sync() error
	}
)

type (
	// productionDependencies is an empty struct that implements all of the
	// dependencies using full featured libraries.
	productionDependencies struct{}
)

// afterDuration gives the host the ability to wait for a certain duration
// before receiving on a channel.
func (productionDependencies) afterDuration(d time.Duration) <-chan time.Time {
	return time.After(d)
}

// createFile gives the host the ability to create files on the operating
// system.
func (productionDependencies) createFile(s string) (file, error) {
	return os.Create(s)
}

// loadFile allows the host to load a persistence structure form disk.
func (productionDependencies) loadFile(m persist.Metadata, i interface{}, s string) error {
	return persist.LoadFile(m, i, s)
}

// mkdirAll gives the host the ability to create chains of folders within the
// filesystem.
func (productionDependencies) mkdirAll(s string, fm os.FileMode) error {
	return os.MkdirAll(s, fm)
}

// newLogger creates a logger that the host can use to log messages and write
// critical statements.
func (productionDependencies) newLogger(s string) (*persist.Logger, error) {
	return persist.NewFileLogger(s)
}

// randRead fills the input bytes with random data.
func (productionDependencies) randRead(b []byte) (int, error) {
	return rand.Read(b)
}
