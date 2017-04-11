package contractor

import (
	"sort"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
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
			lastSuccess = host.ScanHistory[i].Timestamp
			break
		}
	}
	return lastScan.Sub(lastSuccess) >= missingWindow
}

// markBadContracts will go through the contractors set of contracts and mark
// any of the contracts which are no longer performing well.
func (c *Contractor) markBadContracts() {
	// The hosts will be compared against the hosts in the hostdb to determine
	// whether they have an acceptable score or if they should be replaced.
	// Determine what counts as an acceptable score.
	hosts := c.hdb.RandomHosts(int(c.allowance.Hosts), nil)
	// Find the host of the bunch with the lowest score.
	var lowestScore types.Currency
	if len(hosts) > 0 {
		// Get the score of the first host, as a baseline.
		lowestScore = c.hdb.ScoreBreakdown(hosts[0]).Score
		// Find the lowest score overall.
		for _, host := range hosts {
			if c.hdb.ScoreBreakdown(host).Score.Cmp(lowestScore) < 0 {
				lowestScore = c.hdb.ScoreBreakdown(host).Score
			}
		}
	}
	// Adjust the lowest score further down. This gives the hosts wiggle room to
	// not need to be exactly the best performers in order to be acceptable.
	lowestScore = lowestScore.Div64(badScoreForgiveeness)


	// Check each host for various conditions that would cause them to be
	// considered 'bad' hosts.
	for i, contract := range c.contracts {
		if !contract.InGoodStanding {
			// Contract has already been marked as a bad contract, no need to
			// check again. It's done for.
			continue
		}
		if c.isMissing(contract.ID) {
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
		if c.hdb.ScoreBreakdown(host).Score.Cmp(lowestScore) < 0 {
			// Host's score is unaccepably low.
			c.contracts[i].InGoodStanding = false
			continue
		}
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

	// Only one round of contract repair should be running at a time.
	if !c.contractRepairLock.TryLock() {
		return
	}
	defer c.contractRepairLock.Unlock()

	// Sanity check - verify that there is at most one contract per host in
	// c.contracts.
	if build.DEBUG {
		c.mu.Lock()
		hosts := make(map[string]struct{})
		for _, contract := range c.contracts {
			_, exists := hosts[string(contract.HostPublicKey.Key)]
			if exists {
				build.Critical("Contractor has multiple contracts for the same host.")
			}
			hosts[string(contract.HostPublicKey.Key)] = struct{}{}
		}
		c.mu.Unlock()
	}

	// Reveiw the set of contracts held by the contractor, and mark any
	// contracts whose hosts have fallen out of favor.
	c.mu.Lock()
	c.markBadContracts()
	c.mu.Unlock()

	// Iterate through the set of contracts and find any that need to be renewed
	// due to low funds or upcoming expiration.
	numGoodContracts := 0
	for i, contract := range c.contracts {
		// Exit if stop has been called.
		select {
			case <-c.tg.StopChan()
				return
			default:
		}

		// Skip bad contracts.
		if !contract.InGoodStanding {
			continue
		}

		// Check that the contract is not empty.
		empty := c.RenterFunds().Cmp(emptiestAcceptableContract) <= 0

		// Check that the contract is not expiring soon.
		expiring := c.blockHeight+c.allowance.RenewWindow >= contract.EndHeight()

		if empty || expiring {
			// TODO: Call renew on the contract.
		}
		numGoodContracts++
	}

	// Form any extra contracts if needed.
	for numGoodContracts < c.allowance.Hosts {
		// Exit if stop has been called.
		select {
			case <-c.tg.StopChan()
				return
			default:
		}

		// TODO: Form a new contract
		numGoodContracts++
	}
}
