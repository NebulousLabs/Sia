package host

import (
	"errors"
	"os"

	"github.com/NebulousLabs/Sia/persist"
)

// Fake errors that get returned when a simulated failure of a dependency is
// desired for testing.
var (
	mockErrLoadFile     = errors.New("simulated LoadFile failure")
	mockErrMkdirAll     = errors.New("simulated MkdirAll failure")
	mockErrNewLogger    = errors.New("simulated NewLogger failure")
	mockErrOpenDatabase = errors.New("simulated OpenDatabase failure")
)

// These interfaces define the Host's dependencies. Mocking implementation
// complexity can be reduced by defining each dependency as the minimium
// possible subset of the real dependency.
type (
	// dependencies defines all of the dependencies of the Host.
	dependencies interface {
		// LoadFile allows the host to load a persistence structure form disk.
		LoadFile(persist.Metadata, interface{}, string) error

		// MkdirAll gives the host the ability to create chains of folders
		// within the filesystem.
		MkdirAll(string, os.FileMode) error

		// NewLogger creates a logger that the host can use to log messages and
		// write critical statements.
		NewLogger(string) (*persist.Logger, error)

		// OpenDatabase creates a database that the host can use to interact
		// with large volumes of persistent data.
		OpenDatabase(persist.Metadata, string) (*persist.BoltDatabase, error)
	}
)

type (
	// productionDependencies is an empty struct that implements all of the
	// dependencies using full featured libraries.
	productionDependencies struct{}
)

// MkdirAll gives the host the ability to create chains of folders within the
// filesystem.
func (productionDependencies) MkdirAll(s string, fm os.FileMode) error {
	return os.MkdirAll(s, fm)
}

// NewLogger creates a logger that the host can use to log messages and write
// critical statements.
func (productionDependencies) NewLogger(s string) (*persist.Logger, error) {
	return persist.NewLogger(s)
}

// OpenDatabase creates a database that the host can use to interact with large
// volumes of persistent data.
func (productionDependencies) OpenDatabase(m persist.Metadata, s string) (*persist.BoltDatabase, error) {
	return persist.OpenDatabase(m, s)
}

// LoadFile allows the host to load a persistence structure form disk.
func (productionDependencies) LoadFile(m persist.Metadata, i interface{}, s string) error {
	return persist.LoadFile(m, i, s)
}
