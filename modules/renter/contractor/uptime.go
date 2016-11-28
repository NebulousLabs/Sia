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
			return 24 * time.Hour // 1 day
		case "standard":
			return 7 * 24 * time.Hour // 1 week
		case "testing":
			return 1 * time.Minute // 1 minute
		}
		panic("undefined uptimeWindow")
	}()
)

// isOffline decides whether a host should be considered offline, based on its
// scan metrics.
func isOffline(host modules.HostDBEntry) bool {
	// consider a host offline if:
	// 1) it has been scanned at least 3 times within the uptime window, and
	// 2) the 3 most recent scans all failed
	var lastWeek []modules.HostDBScan
	for _, scan := range host.ScanHistory {
		if time.Since(scan.Timestamp) < uptimeWindow {
			lastWeek = append(lastWeek, scan)
		}
	}
	// need at least 3 scans to make a fair judgment
	if len(lastWeek) < 3 {
		return false
	}
	// return true if all 3 most recent scans failed
	// NOTE: lastWeek is ordered from oldest to newest
	lastWeek = lastWeek[len(lastWeek)-3:]
	return !lastWeek[0].Success && !lastWeek[1].Success && !lastWeek[2].Success
}

// threadedMonitorUptime regularly checks host uptime, and deletes contracts
// whose hosts fall below a minimum uptime threshold.
func (c *Contractor) threadedMonitorUptime() {
	for range time.Tick(uptimeInterval) {
		// get current contracts
		contracts := c.Contracts()

		// identify hosts with poor uptime
		var badContracts []modules.RenterContract
		for _, contract := range contracts {
			host, ok := c.hdb.Host(contract.NetAddress)
			if !ok {
				c.log.Printf("WARN: missing host entry for %v", contract.NetAddress)
				continue
			}
			if isOffline(host) {
				badContracts = append(badContracts, contract)
			}
		}
		if len(badContracts) == 0 {
			// nothing to do
			continue
		}

		// delete contracts with bad hosts. When a new block arrives,
		// ProcessConsensusChange will take care of forming new contracts as
		// needed.
		c.mu.Lock()
		for _, contract := range badContracts {
			delete(c.contracts, contract.ID)
		}
		c.mu.Unlock()
		c.log.Printf("INFO: deleted %v contracts because their hosts were offline", len(badContracts))
	}
}
