package contractor

import (
	"github.com/NebulousLabs/Sia/modules"
)

// isMissing indicates whether a host has been offline for long enough to be
// considered missing, based on the hostdb scan metrics.
func (c *Contractor) isMissing(contract modules.RenterContract) bool {
	// If the host is not in the hostdb, then the host is missing.
	host, exists := c.hdb.Host(contract.HostPublicKey)
	if !exists {
		return true
	}

	// Sanity check - ScanHistory should always be ordered from oldest to
	// newest.
	if build.DEBUG && !sort.IsSorted(host.ScanHistory) {
		sort.Sort(host.ScanHistory)
		build.Critical("host's scan history was not sorted")
	}

	// Consider a host offline if:
	// 1) The host has been scanned enough times.
	// 2) The most recent scans have all failed.
	// 3) The time between the most recent scan and the last successful scan
	//    (or first scan) has exceeded the acceptable amount.
	numScans := len(host.ScanHistory)
	if numScans < missingMinScans {
		// Not enough data to make a fair judgment.
		return false
	}
	recent := host.ScanHistory[numScans-missingMinScans:]
	for _, scan := range recent {
		if scan.Success {
			// One of the recent scans succeeded.
			return false
		}
	}

	// Initialize window bounds.
	lastScan := host.ScanHistory[0].Timestamp
	lastSuccess := host.ScanHistory[numScans-1].Timestamp
	// Iterate from newest-oldest, seeking to last successful scan.
	for i := numScans - 1; i >= 0; i-- {
		if host.ScanHistory[i].Success {
			lastSuccessful = host.ScanHistory[i].Success
			break
		}
	}
	return lastScan.Sub(lastSuccess) >= missingWindow
}

// markBadContracts will go through the contractors set of contracts and mark
// any of the contracts which are no longer performing well.
func (c *Contractor) markBadContracts() {
	for i, contract := range c.contracts {
		if !contract.InGoodStanding {
			// Contract has already been marked as a bad contract, no need to
			// check again. It's done for.
			continue
		}
		if contract.isMissing(contract.ID) {
			// Host has been offline for long enough to fall out of good
			// standing.
			c.contracts[i].InGoodStanding = false
			continue
		}

		// Check whether the contract still has an acceptable score in the
		// hostdb.
		host, exists := c.hdb.Host(contract.HostPublicKey)
		if !exists {
			c.contracts[i].InGoodStanding = false
			continue
		}
		// TODO: Figure out what an acceptable score is.
		// TODO: Host is not in good standing if it does not have that score.
	}
}

// threadedRepairContracts checks the status of the contracts that the
// contractor has, refreshing contracts that are running out money, renewing
// contracts that are expiring, pruning contracts if the corresponding hosts
// have become unfavorable, and forming new contract if too many hosts have
// been pruned.
func (c *Contractor) threadedRepairContracts() {
	err := c.tg.Add()
	if err != nil {
		return
	}
	defer c.tg.Done()

	// Only one round of contract repair should to be running at a time.
	if !c.editLock.TryLock() {
		return
	}
	defer c.editLock.Unlock()

	// Reveiw the set of contracts held by the contractor, and mark any
	// contracts whose hosts have fallen out of favor.
	c.mu.Lock()
	c.markBadContracts()
	c.mu.Unlock()

	// TODO: Iterate through the good contracts and identify whether any need to
	// be renewed. This could be do to timing, and could be due to running out
	// of funds.

	// TODO: Determine whether another contract is necessary.
}
