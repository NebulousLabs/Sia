package modules

import (
	"time"

	"github.com/NebulousLabs/Sia/build"
)

var (
	SafeMutexDelay time.Duration
)

func init() {
	if build.Release == "dev" {
		SafeMutexDelay = 3 * time.Second
	} else if build.Release == "standard" {
		SafeMutexDelay = 60 * time.Second
	} else if build.Release == "testing" {
		SafeMutexDelay = 1 * time.Second
	}
}
