package persist

import (
	"path/filepath"

	"github.com/mitchellh/go-homedir"

	"github.com/NebulousLabs/Sia/build"
)

var HomeFolder = func() string {
	// use a special folder during testing
	if build.Release == "testing" {
		return filepath.Join(build.SiaTestingDir, "home")
	}

	home, err := homedir.Dir()
	if err != nil {
		println("could not find homedir: " + err.Error()) // TODO: better error handling
		return ""
	}
	return filepath.Join(home, ".config", "Sia")
}()
