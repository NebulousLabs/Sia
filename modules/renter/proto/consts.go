package proto

import (
	"time"

	"github.com/NebulousLabs/Sia/build"
)

var (
	// The amount of time that a host has to respond to a dial.
	dialHostTimeout = build.Select(build.Var{
		Standard: time.Second * 45,
		Dev:      time.Second * 15,
		Testing:  time.Second * 3,
	}).(time.Duration)
)
