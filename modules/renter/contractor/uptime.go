package contractor

import (
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// uptimeMinScans is the minimum number of scans required to judge whether a
// host is offline or not.
const uptimeMinScans = 3

// uptimeWindow specifies the duration in which host uptime is checked.
var uptimeWindow = func() time.Duration {
	switch build.Release {
	case "dev":
		return 30 * time.Minute
	case "standard":
		return 7 * 24 * time.Hour // 1 week.
	case "testing":
		return 15 * time.Second
	}
	panic("undefined uptimeWindow")
}()

// IsOffline indicates whether a contract's host should be considered offline,
// based on its scan metrics.
func (c *Contractor) IsOffline(pk types.SiaPublicKey) bool {
	host, ok := c.hdb.Host(pk)
	if !ok {
		// No host, assume offline.
		return true
	}
	return isOffline(host)
}

// isOffline indicates whether a host should be considered offline, based on
// its scan metrics.
func isOffline(host modules.HostDBEntry) bool {
	// See if the host has a scan history.
	if len(host.ScanHistory) < 1 {
		// No scan history, assume offline.
		return true
	}
	// If we only have one scan in the history we return false if it was
	// successful.
	if len(host.ScanHistory) == 1 {
		return !host.ScanHistory[0].Success
	}
	// Otherwise we use the last 2 scans. This way a short connectivity problem
	// won't mark the host as offline.
	success1 := host.ScanHistory[len(host.ScanHistory)-1].Success
	success2 := host.ScanHistory[len(host.ScanHistory)-2].Success
	return !(success1 || success2)
}
