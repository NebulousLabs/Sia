package hostdb

import (
	"time"

	"github.com/NebulousLabs/Sia/build"
)

const (
	defaultScanSleep = 1*time.Hour + 37*time.Minute
	maxScanSleep     = 4 * time.Hour
	minScanSleep     = 1*time.Hour + 20*time.Minute

	maxSettingsLen = 4e3

	hostRequestTimeout = 60 * time.Second
	hostScanDeadline   = 240 * time.Second

	// saveFrequency defines how frequently the hostdb will save to disk. Hostdb
	// will also save immediately prior to shutdown.
	saveFrequency = 2 * time.Minute
)

var (
	// hostCheckupQuantity specifies the number of hosts that get scanned every
	// time there is a regular scanning operation.
	hostCheckupQuantity = build.Select(build.Var{
		Standard: int(250),
		Dev:      int(6),
		Testing:  int(5),
	}).(int)

	// scanningThreads is the number of threads that will be probing hosts for
	// their settings and checking for reliability.
	scanningThreads = build.Select(build.Var{
		Standard: int(40),
		Dev:      int(4),
		Testing:  int(3),
	}).(int)
)
