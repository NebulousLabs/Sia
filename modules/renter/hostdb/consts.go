package hostdb

import (
	"time"

	"github.com/NebulousLabs/Sia/build"
)

const (
	// defaultScanSleep is the amount of time that the hostdb will sleep if it
	// cannot successfully get a random number.
	defaultScanSleep = 1*time.Hour + 37*time.Minute

	// maxScanSleep is the maximum amount of time that the hostdb will sleep
	// between performing scans of the hosts.
	maxScanSleep = 4 * time.Hour

	// minScanSleep is the minimum amount of time that the hostdb will sleep
	// between performing scans of the hosts.
	minScanSleep = 1*time.Hour + 20*time.Minute

	// maxSettingsLen indicates how long in bytes the host settings field is
	// allowed to be before being ignored as a DoS attempt.
	maxSettingsLen = 10e3

	// hostRequestTimeout indicates how long a host has to respond to a dial.
	hostRequestTimeout = 2 * time.Minute

	// hostScanDeadline indicates how long a host has to complete an entire
	// scan.
	hostScanDeadline = 4 * time.Minute

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
