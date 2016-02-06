package host

import (
	"errors"
	"os"

	"github.com/NebulousLabs/Sia/persist"
)

var (
	// errMkdirAllMock is a fake error that is returned when a simulated
	// failure of MkdirAll is desired for testing.
	errMkdirAllMock = errors.New("simulated MkdirAll failure")

	// errNewLoggerMock is a fake error that is returned when a simulated
	// failure of NewLogger is desired for testing.
	errNewLoggerMock = errors.New("simulated NewLogger failure")
)

// These interfaces define the Host's dependencies. Mocking implementation
// complexity can be reduced by defining each dependency as the minimium
// possible subset of the real dependency.
type (
	// dependencies defines all of the dependencies of the Host.
	dependencies interface {
		// MkdirAll gives the host the ability to create chains of folders
		// within the filesystem.
		MkdirAll(string, os.FileMode) error

		// NewLogger creates a logger that the host can use to log messages and
		// write critical statements.
		NewLogger(string) (*persist.Logger, error)
	}

	// stub is an empty struct which naively implements all dependencies of the
	// host. It can be embedded into specialized structs during host testing to
	// eliminate most of the legwork that is required when mocking.
	stub struct{}
)

// MkdirAll gives the host the ability to create chains of folders within the
// filesystem.
func (stub) MkdirAll(string, os.FileMode) error { return nil }

// NewLogger creates a logger that the host can use to log messages and write
// critical statements.
func (stub) NewLogger(string) (*persist.Logger, error) { return &persist.Logger{}, nil }
