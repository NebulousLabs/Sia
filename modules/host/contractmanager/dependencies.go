package contractmanager

import (
	"crypto/rand"
	"errors"
	"io/ioutil"
	"os"
	"strings"

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

		// readFile reads a file in full from the filesystem.
		readFile(string) ([]byte, error)

		// removeFile removes a file from file filesystem.
		removeFile(string) error

		// writeFile writes data to the filesystem using the provided filename.
		writeFile(string, []byte, os.FileMode) error
	}
)

type (
	// productionDependencies is an empty struct that implements all of the
	// dependencies using full featured libraries.
	productionDependencies struct{}
)

// composeErrors will take two errors and compose them into a single errors
// with a longer message. Any nil errors used as inputs will be stripped out,
// and if there are zero non-nil inputs then 'nil' will be returned.
func composeErrors(errs ...error) error {
	// Strip out any nil errors.
	var errStrings []string
	for _, err := range errs {
		if err != nil {
			errStrings = append(errStrings, err.Error())
		}
	}

	// Return nil if there are no non-nil errors in the input.
	if len(errStrings) <= 0 {
		return nil
	}

	// Combine all of the non-nil errors into one larger return value.
	return errors.New(strings.Join(errStrings, "; "))
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

// readFile reads a file from the filesystem.
func (productionDependencies) readFile(s string) ([]byte, error) {
	return ioutil.ReadFile(s)
}

// removeFile removes a file from the filesystem.
func (productionDependencies) removeFile(s string) error {
	return os.Remove(s)
}

// writeFile writes a file to the filesystem.
func (productionDependencies) writeFile(s string, b []byte, fm os.FileMode) error {
	return ioutil.WriteFile(s, b, fm)
}
