package modules

import (
	"log"
	"time"

	"github.com/NebulousLabs/Sia/build"
)

var (
	LogSettings    = log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile
	SafeMutexDelay time.Duration
)

func init() {
	if build.Release == "dev" {
		SafeMutexDelay = 14 * time.Second
	} else if build.Release == "standard" {
		SafeMutexDelay = 120 * time.Second
	} else if build.Release == "testing" {
		SafeMutexDelay = 6 * time.Second
	}
}
