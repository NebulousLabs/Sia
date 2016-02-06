package host

import (
	"os"

	"github.com/NebulousLabs/Sia/persist"
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
