package contractmanager

import (
	"errors"
	"io"
	"os"
	"sync"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/fastrand"
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

		// destruct will clean up the dependencies, panicking if there are
		// unclosed resources.
		destruct()

		// disrupt is a general purpose testing function which will return true
		// if a disruption is happening and false if a disruption is not. Most
		// frequently it is used to simulate power-failures by forcing some of
		// the code to terminate partway through. The string input can be used
		// by the testing code to distinguish between the many places where
		// production code can be disrupted.
		disrupt(string) bool

		// Init performs any necessary initialization for the set of
		// dependencies.
		init()

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

		// removeFile removes a file from disk.
		removeFile(string) error

		// renameFile renames a file on disk to another name.
		renameFile(string, string) error
	}

	// file implements all of the methods that can be called on an os.File.
	file interface {
		io.ReadWriteCloser
		Name() string
		ReadAt([]byte, int64) (int, error)
		Sync() error
		Truncate(int64) error
		WriteAt([]byte, int64) (int, error)
	}
)

type (
	// productionDependencies implements all of the dependencies using full
	// featured libraries.
	productionDependencies struct {
		shouldInit bool
		openFiles  map[string]int
		mu         *sync.Mutex
	}

	// productionFile allows the production dependencies to track
	productionFile struct {
		pd *productionDependencies
		*os.File
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
		coin := fastrand.Intn(2)
		if coin == 0 {
			break
		}
	}
	return val

}

// createFile gives the host the ability to create files on the operating
// system.
func (pd *productionDependencies) createFile(s string) (file, error) {
	if !build.DEBUG {
		return os.Create(s)
	}

	f, err := os.Create(s)
	if err != nil {
		return f, err
	}

	pd.mu.Lock()
	v := pd.openFiles[s]
	pd.openFiles[s] = v + 1
	pd.mu.Unlock()
	return &productionFile{
		pd:   pd,
		File: f,
	}, nil
}

// destruct checks that all resources have been cleaned up correctly.
func (pd *productionDependencies) destruct() {
	if !build.DEBUG {
		return
	}

	pd.mu.Lock()
	l := len(pd.openFiles)
	pd.mu.Unlock()
	if l != 0 {
		panic("unclosed resources - most likely file handles")
	}
}

// disrupt will always return false when using the production dependencies,
// because production code should never be intentionally disrupted.
func (productionDependencies) disrupt(string) bool {
	return false
}

// init will create the map and mutex
func (pd *productionDependencies) init() {
	if !build.DEBUG {
		return
	}

	if !pd.shouldInit {
		pd.shouldInit = true
		pd.openFiles = make(map[string]int)
		pd.mu = new(sync.Mutex)
	}
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
func (pd *productionDependencies) openFile(s string, i int, fm os.FileMode) (file, error) {
	if !build.DEBUG {
		return os.OpenFile(s, i, fm)
	}

	f, err := os.OpenFile(s, i, fm)
	if err != nil {
		return f, err
	}

	pd.mu.Lock()
	v := pd.openFiles[s]
	pd.openFiles[s] = v + 1
	pd.mu.Unlock()
	return &productionFile{
		pd:   pd,
		File: f,
	}, nil
}

// removeFile will remove a file from disk.
func (pd *productionDependencies) removeFile(s string) error {
	if !build.DEBUG {
		return os.Remove(s)
	}

	pd.mu.Lock()
	v, exists := pd.openFiles[s]
	pd.mu.Unlock()
	if exists && v > 0 {
		return errors.New("cannot remove the file, it's open somewhere else right now")
	}
	return os.Remove(s)
}

// renameFile renames a file on disk.
func (pd *productionDependencies) renameFile(s1 string, s2 string) error {
	if !build.DEBUG {
		return os.Rename(s1, s2)
	}

	pd.mu.Lock()
	v1, exists1 := pd.openFiles[s1]
	v2, exists2 := pd.openFiles[s2]
	pd.mu.Unlock()
	if exists1 && v1 > 0 {
		return errors.New("cannot remove the file, it's open somewhere else right now")
	}
	if exists2 && v2 > 0 {
		return errors.New("cannot remove the file, it's open somewhere else right now")
	}
	return os.Rename(s1, s2)
}

// Close will close a file, checking whether the file handle is open somewhere
// else before closing completely. This check is performed on Windows but not
// Linux, therefore a mock is used to ensure that linux testing picks up
// potential problems that would be seen on Windows.
func (pf *productionFile) Close() error {
	if !build.DEBUG {
		return pf.File.Close()
	}

	pf.pd.mu.Lock()
	v, exists := pf.pd.openFiles[pf.File.Name()]
	if !exists {
		panic("file not registered")
	}
	if v == 1 {
		delete(pf.pd.openFiles, pf.File.Name())
	} else if v > 1 {
		pf.pd.openFiles[pf.File.Name()] = v - 1
	} else {
		panic("inconsistent state")
	}
	pf.pd.mu.Unlock()
	return pf.File.Close()
}
