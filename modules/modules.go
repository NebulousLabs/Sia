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
		SafeMutexDelay = 25 * time.Second
	} else if build.Release == "standard" {
		SafeMutexDelay = 60 * time.Second
	} else if build.Release == "testing" {
		SafeMutexDelay = 10 * time.Second
	}
}
