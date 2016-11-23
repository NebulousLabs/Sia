package contractor

import (
	"time"

	"github.com/NebulousLabs/Sia/build"
)

// uptimeInterval specifies how frequently hosts are checked.
var uptimeInterval = func() time.Duration {
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

// threadedMonitorUptime regularly checks host uptime, and deletes contracts
// whose hosts fall below a minimum uptime threshold.
func (c *Contractor) threadedMonitorUptime() {
	for range time.Tick(uptimeInterval) {
		// get current contracts
		contracts := c.Contracts()

		// identify hosts with poor uptime
		badContracts := contracts[:0] // reuse memory during filter
		for _, contract := range contracts {
			if c.hdb.IsOffline(contract.NetAddress) {
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
		c.editLock.Lock()
		c.mu.Lock()
		for _, contract := range badContracts {
			delete(c.contracts, contract.ID)
		}
		c.mu.Unlock()
		c.editLock.Unlock()
		c.log.Printf("INFO: deleted %v contracts because their hosts were offline", len(badContracts))
	}
}
