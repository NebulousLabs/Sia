package main

import (
	"github.com/NebulousLabs/Andromeda/siacore"
)

type daemon struct {
	core *siacore.Environment
}

func createDaemon() (d daemon, err error) {
	d = new(daemon)
	d.core, err = siacore.CreateEnvironment()
	if err != nil {
		return
	}

	return
}
