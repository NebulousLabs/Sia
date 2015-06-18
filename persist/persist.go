package persist

import (
	"path/filepath"

	"github.com/mitchellh/go-homedir"

	"github.com/NebulousLabs/Sia/build"
)

var HomeFolder = func() string {
	home, err := homedir.Dir()
	if err != nil {
		println("could not find homedir: " + err.Error()) // TODO: better error handling
		return ""
	}

	switch build.Release {
	case "testing":
		return filepath.Join(build.SiaTestingDir, "home")
	case "dev":
		return filepath.Join(home, ".config", "SiaDev")
	default:
		return filepath.Join(home, ".config", "Sia")
	}
}()
