package persist

import (
	"path/filepath"

	"github.com/mitchellh/go-homedir"
)

var (
	HomeFolder string
)

func init() {
	home, err := homedir.Dir()
	if err != nil {
		println("could not find homedir: " + err.Error()) // TODO: better error handling
	}
	HomeFolder = filepath.Join(home, ".config", "Sia")
}
