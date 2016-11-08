package contractmanager

import (
	"errors"
	"io"
	"os"
	"sync"

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
		// changes.
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

		// openFile opens a file for the host.
		openFile(string, int, os.FileMode) (file, error)
	}

	// file implements all of the methods that can be called on an os.File.
	file interface {
		io.ReadWriteCloser
		sync.Locker
		Name() string
		Seek(int64, int) (int64, error)
		Sync() error
		Truncate(int64) error
	}
)

type (
	// productionDependencies is an empty struct that implements all of the
	// dependencies using full featured libraries.
	productionDependencies struct{}

	// lockerFile extends the os.File type to allow the file to be locked and
	// unlocked.
	lockerFile struct {
		*os.File
		sync.Mutex
	}
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
	osFile, err := os.Create(s)
	if err != nil {
		return nil, err
	}
	return &lockerFile{
		File: osFile,
	}, nil
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

// openFile opens a file for the contract manager.
func (productionDependencies) openFile(s string, i int, fm os.FileMode) (file, error) {
	osFile, err := os.OpenFile(s, i, fm)
	if err != nil {
		return nil, err
	}
	return &lockerFile{
		File: osFile,
	}, nil
}
