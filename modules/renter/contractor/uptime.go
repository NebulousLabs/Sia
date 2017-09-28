package contractor

import (
	"sort"
	"time"

	"github.com/NebulousLabs/Sia/build"
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
func (c *Contractor) IsOffline(id types.FileContractID) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isOffline(id)
}

// isOffline indicates whether a contract's host should be considered offline,
// based on its scan metrics.
func (c *Contractor) isOffline(id types.FileContractID) bool {
	// Fetch the corresponding contract in the contractor. If the most recent
	// contract is not in the contractors set of active contracts, this contract
	// line is dead, and thus the contract should be considered 'offline'.
	contract, ok := c.contracts[id]
	if !ok {
		return true
	}
	host, ok := c.hdb.Host(contract.HostPublicKey)
	if !ok {
		return true
	}
	if len(host.ScanHistory) < 1 {
		return true
	}
	return host.ScanHistory[len(host.ScanHistory)-1].Success
}
