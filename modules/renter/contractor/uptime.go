package contractor

import (
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
)

// uptimeInterval specifies how frequently hosts are checked.
var (
	uptimeInterval = func() time.Duration {
		switch build.Release {
		case "dev":
			return 1 * time.Minute
		case "standard":
			return 30 * time.Minute
		case "testing":
			return 100 * time.Millisecond
		}
		panic("undefined uptimeInterval")
	}()

	// uptimeWindow specifies the duration in which host uptime is checked.
	uptimeWindow = func() time.Duration {
		switch build.Release {
		case "dev":
			return 30 * time.Minute
		case "standard":
			return 7 * 24 * time.Hour // 1 week
		case "testing":
			return 15 * time.Second
		}
		panic("undefined uptimeWindow")
	}()
)

// isOffline decides whether a host should be considered offline, based on its
// scan metrics.
func isOffline(host modules.HostDBEntry) bool {
	// consider a host offline if:
	// 1) The host has been scanned at least 3 times in the past uptimeWindow, and
	// 2) The scans occurred over a period of at least 1/3 the uptimeWindow, and
	// 3) None of the scans succeeded
	var pastWeek []modules.HostDBScan
	for _, scan := range host.ScanHistory {
		if time.Since(scan.Timestamp) < uptimeWindow {
			if scan.Success {
				// at least one scan in the uptimeWindow succeeded
				return false
			}
			pastWeek = append(pastWeek, scan)
		}
	}
	if len(pastWeek) < 3 {
		// not enough data to make a fair judgment
		return false
	}
	if pastWeek[len(pastWeek)-1].Timestamp.Sub(pastWeek[0].Timestamp) < uptimeWindow/3 {
		// scans were not sufficiently far apart to make a fair judgment
		return false
	}
	// all conditions satisfied
	return true
}

// threadedMonitorUptime regularly checks host uptime, and deletes contracts
// whose hosts fall below a minimum uptime threshold.
func (c *Contractor) onlineContracts() []modules.RenterContract {
	var cs []modules.RenterContract
	for _, contract := range c.contracts {
		host, ok := c.hdb.Host(contract.NetAddress)
		if !ok {
			c.log.Printf("WARN: missing host entry for %v", contract.NetAddress)
			continue
		}
		// only returns contracts not marked as being offline
		if !isOffline(host) {
			cs = append(cs, contract)
		}
	}
	return cs
}
