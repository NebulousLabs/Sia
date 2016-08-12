package contractmanager

import (
	"crypto/rand"
	"errors"
	"io"
	"os"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
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
		// atLeastOne will return a value that is at least one. In production,
		// the value should always be one. This function is used to test the
		// idempotency of actions, so during testing sometimes the value
		// returned will be higher, causing an idempotent action to be
		// committed multiple times. If the action is truly idempotent,
		// committing it multiple times should not cause any problems or
		// changes. The function is created as a dependency so that it can be
		// hijacked during testing to provide non-probabilistic outcomes.
		atLeastOne() uint64

		// createFile gives the host the ability to create files on the
		// operating system.
		createFile(string) (file, error)

		// disrupt is a general purpose testing function which will return true
		// if a disruption is happening and false if a disruption is not. Most
		// frequently it is used to simulate power-failures by forcing some of
		// the code to terminate partway through. The string input can be used
		// by the testing code to distinguish between the many places where
		// production code can be disrupted.
		disrupt(string) bool

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
		io.Reader
		io.Writer
		Seek(int64, int) (int64, error)
		Sync() error
	}
)

type (
	// productionDependencies is an empty struct that implements all of the
	// dependencies using full featured libraries.
	productionDependencies struct{}
)

// atLeastOne will return a value that is equal to 1 if debugging is disabled.
// If debugging is enabled, a higher value may be returned.
func (productionDependencies) atLeastOne() uint64 {
	if !build.DEBUG {
		return 1
	}

	// Probabilistically return a number greater than one.
	var val uint64
	for {
		val++
		coin, err := crypto.RandIntn(2)
		if err != nil {
			panic(err)
		}
		if coin == 0 {
			break
		}
	}
	return val

}

// createFile gives the host the ability to create files on the operating
// system.
func (productionDependencies) createFile(s string) (file, error) {
	return os.Create(s)
}

// disrupt will always return false when using the production dependencies,
// because production code should never be intentionally disrupted.
func (productionDependencies) disrupt(string) bool {
	return false
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
