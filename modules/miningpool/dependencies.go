package pool

import (
	"errors"
	"io/ioutil"
	"net"
	"os"

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

// These interfaces define the Pool's dependencies. Mocking implementation
// complexity can be reduced by defining each dependency as the minimum
// possible subset of the real dependency.
type (
	// dependencies defines all of the dependencies of the Pool.
	dependencies interface {
		// disrupt can be inserted in the code as a way to inject problems,
		// such as a network call that take 10 minutes or a disk write that
		// never completes. disrupt will return true if the disruption is
		// forcibly triggered. In production, disrupt will always return false.
		disrupt(string) bool

		// listen gives the host the ability to receive incoming connections.
		listen(string, string) (net.Listener, error)

		// loadFile allows the host to load a persistence structure form disk.
		loadFile(persist.Metadata, interface{}, string) error

		// mkdirAll gives the host the ability to create chains of folders
		// within the filesystem.
		mkdirAll(string, os.FileMode) error

		// newLogger creates a logger that the host can use to log messages and
		// write critical statements.
		newLogger(string) (*persist.Logger, error)

		// openDatabase creates a database that the host can use to interact
		// with large volumes of persistent data.
		openDatabase(persist.Metadata, string) (*persist.BoltDatabase, error)

		// randRead fills the input bytes with random data.
		randRead([]byte) (int, error)

		// readFile reads a file in full from the filesystem.
		readFile(string) ([]byte, error)

		// removeFile removes a file from file filesystem.
		removeFile(string) error

		// symlink creates a sym link between a source and a destination.
		symlink(s1, s2 string) error

		// writeFile writes data to the filesystem using the provided filename.
		writeFile(string, []byte, os.FileMode) error
	}
)

type (
	// productionDependencies is an empty struct that implements all of the
	// dependencies using full featured libraries.
	productionDependencies struct{}
)

// disrupt will always return false, but can be over-written during testing to
// trigger disruptions.
func (productionDependencies) disrupt(string) bool {
	return false
}

// listen gives the host the ability to receive incoming connections.
func (productionDependencies) listen(s1, s2 string) (net.Listener, error) {
	return net.Listen(s1, s2)
}

// loadFile allows the host to load a persistence structure form disk.
func (productionDependencies) loadFile(m persist.Metadata, i interface{}, s string) error {
	return persist.LoadJSON(m, i, s)
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

// openDatabase creates a database that the host can use to interact with large
// volumes of persistent data.
func (productionDependencies) openDatabase(m persist.Metadata, s string) (*persist.BoltDatabase, error) {
	return persist.OpenDatabase(m, s)
}

// randRead fills the input bytes with random data.
func (productionDependencies) randRead(b []byte) (int, error) {
	return fastrand.Reader.Read(b)
}

// readFile reads a file from the filesystem.
func (productionDependencies) readFile(s string) ([]byte, error) {
	return ioutil.ReadFile(s)
}

// removeFile removes a file from the filesystem.
func (productionDependencies) removeFile(s string) error {
	return os.Remove(s)
}

// symlink creates a symlink between a source and a destination file.
func (productionDependencies) symlink(s1, s2 string) error {
	return os.Symlink(s1, s2)
}

// writeFile writes a file to the filesystem.
func (productionDependencies) writeFile(s string, b []byte, fm os.FileMode) error {
	return ioutil.WriteFile(s, b, fm)
}
