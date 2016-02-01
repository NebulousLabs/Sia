package modules

import (
	"log"
	"time"

	"github.com/NebulousLabs/Sia/build"
)

var (
	// LogSettings is the recommended settings for logging. This value is
	// DEPRECATED. Instead, the persist.Logger should be used.
	LogSettings = log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile

	// SafeMutexDelay is the recommended timeout for the deadlock detecting
	// mutex. This value is DEPRECATED, as safe mutexes are no longer
	// recommended. Instead, the locking conventions should be followed and a
	// traditional mutex or a demote mutex should be used.
	SafeMutexDelay time.Duration
)

func init() {
	if build.Release == "dev" {
		SafeMutexDelay = 40 * time.Second
	} else if build.Release == "standard" {
		SafeMutexDelay = 60 * time.Second
	} else if build.Release == "testing" {
		SafeMutexDelay = 20 * time.Second
	}
}
