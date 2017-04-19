package contractor

import (
	"sort"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

// managedLockContract will block until a contract is available, then will grab an
// exclusive lock on the contract.
func (c *Contractor) managedLockContract(id types.FileContractID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, exists := c.contractLocks[id]
	if !exists {
		c.contractLocks[id] = new(sync.TryMutex)
	}
	c.contractLocks[id].Lock()
}

// managedTryLockContract will attempt to grab a lock on a contract, returning
// immediately if the contract is unavailable.
func (c *Contractor) managedTryLockContract(id types.FileContractID) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, exists := c.contractLocks[id]
	if !exists {
		c.contractLocks[id] = new(sync.TryMutex)
	}
	return c.contractLocks[id].TryLock()
}

// managedUnlockContract will unlock a locked contract.
func (c *Contractor) managedUnlockContract(id types.FileContractID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.contractLocks[id].Unlock()
}

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
	lowestScore = lowestScore.Div64(badScoreForgiveness)

	// Check each host for various conditions that would cause them to be
	contracts := make([]modules.RenterContract, 0, len(c.contracts))
	for _, contract := range c.contracts {
		contracts = append(contracts, contract)
	}
	for _, contract := range contracts {
		if !contract.InGoodStanding {
			// Contract has already been marked as a bad contract, no need to
			// check again. It's done for.
			continue
		}
		if c.isMissing(contract) {
			// Host has been offline for long enough to fall out of good
			// standing.
			contract.InGoodStanding = false
			c.contracts[contract.ID] = contract
			continue
		}

		// Check whether the contract still has an acceptable score in the
		// hostdb.
		host, exists := c.hdb.Host(contract.HostPublicKey)
		if !exists {
			contract.InGoodStanding = false
			c.contracts[contract.ID] = contract
			continue
		}
		if c.hdb.ScoreBreakdown(host).Score.Cmp(lowestScore) < 0 {
			// Host's score is unaccepably low.
			contract.InGoodStanding = false
			c.contracts[contract.ID] = contract
			continue
		}

		// TODO: Contract is not useful for upload if the host is full, contract
		// is not useful for upload if it was formed due to filesharing reasons.

		// TODO: Have another check that figures out whether the host has
		// storage space remaining. If not, host is nonetheless in good standing
		// but is 'not uploadable' or something. Tells the contractor that
		// downloads are okay, but also that this host does not count as one of
		// the 50, and also that uploads will not work.
		//
		// Verify that the contract fee is justified if the host is full. If
		// not, host is also not in good standing.
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

	// TODO: Verify that the hostdb has finished scanning hosts from the
	// original blockchain download. If there are unscanned hosts, it's too
	// early to form contracts.

	// TODO: Verify that consensus has synced. Do not run without it.

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
	var needsRenew []modules.RenterContract
	var numGoodContracts uint64
	func() {
		c.mu.Lock()
		defer c.mu.Unlock()

		for _, contract := range c.contracts {
			// Skip bad contracts.
			if !contract.InGoodStanding {
				continue
			}

			// Grab the host that is being used for this contract.
			host, exists := c.hdb.Host(contract.HostPublicKey)
			if !exists {
				continue // This contract is with a host that doesn't exist.
			}

			// TODO: Empty is also true if the host has run out of collateral.
			//
			// Check that the contract is not empty.
			empty := contract.RenterFunds().Cmp(emptiestAcceptableContract) <= 0
			empty = empty && contract.UsefulForUploading
			// The host is not empty if the host has no more storage remaining.
			if empty && host.RemainingStorage < 10e9 { // TODO: Const.
				empty = false
			}
			// Check that the contract is not expiring soon.
			expiring := c.blockHeight+c.allowance.RenewWindow >= contract.EndHeight()
			if empty || expiring {
				needsRenew = append(needsRenew, contract)
			}

			// Contract counts as good regardless of whether it needs to be
			// renewed.
			numGoodContracts++
		}
	}()

	// Get the height that the contracts should be formed at.
	c.mu.Lock()
	contractsEndHeight := c.blockHeight + c.allowance.Period
	c.mu.Unlock()
	for _, contract := range needsRenew {
		// Exit if stop has been called.
		select {
		case <-c.tg.StopChan():
			return
		default:
		}

		// Grab the host that is being used for renew.
		host, exists := c.hdb.Host(contract.HostPublicKey)
		if !exists {
			continue // This contract is with a host that doesn't exist.
		}

		var newCost types.Currency
		empty := contract.RenterFunds().Cmp(emptiestAcceptableContract) <= 0
		if empty {
			// Contract is being renewed because it ran out of money. Double the
			// amount of money that's allocated to the contract.
			prevCost := contract.TotalCost.Sub(ContractFee).Sub(TxnFee).Sub(SiafundFee)
			newCost = prevCost.Mul64(2) // TODO: Const
		} else {
			// Contract is being renewed because it has hit expiration. Use 33%
			// more funds than were spent last time.
			prevBase := contract.TotalCost.Sub(ContractFee).Sub(TxnFee).Sub(SiafundFee)
			// The amount of money that the contract started with should not be
			// less than the amount of money remaining in the contract, but
			// double check just to be sure.
			if prevBase.Cmp(contract.RenterFunds()) < 0 {
				build.Critical("A contracts base funds is smaller than it's available funds:", prevBase, contract.RenterFunds())
			}
			prevCost := prevBase.Sub(contract.RenterFunds())
			newCost = prevCost.Mul64(4).Div64(3) // TODO: Const
		}

		// Verify that the new cost is at least enough to cover all the existing
		// data and some extra.
		timeExtension := uint64(contractsEndHeight - contract.LastRevision.NewWindowEnd)                                 // TODO: May need to add host.WindowSize too.
		minRequired := host.StoragePrice.Mul64(contract.LastRevision.NewFileSize).Mul64(timeExtension).Mul64(4).Div64(3) // TODO: Const.
		if minRequired.Cmp(newCost) < 0 {
			newCost = minRequired
		}

		// Calculate the desired collateral.
		collateral := newCost.Mul(host.Collateral).Div(host.StoragePrice)
		// Don't exceed the host's maximum collateral.
		if collateral.Cmp(host.MaxCollateral) > 0 {
			collateral = host.MaxCollateral
		}
		// Don't exceed the internal collateral fraction.
		if newCost.Mul64(5).Cmp(collateral) < 0 { // TODO: Const
			collateral = newCost.Mul64(5) // TODO: Const
		}

		c.managedLockContract(contract.ID)
		c.managedRenewContract(contract, host, newCost, collateral, contractsEndHeight)
		c.managedUnlockContract(contract.ID)

		// Soft sleep between contract formation.
		select {
		case <-c.tg.StopChan():
			return
		case <-time.After(sleepFormationInterval):
		}
	}

	// Form any extra contracts if needed.
	c.mu.Lock()
	wantedHosts := c.allowance.Hosts
	c.mu.Unlock()
	for numGoodContracts < wantedHosts {
		// Exit if stop has been called.
		select {
		case <-c.tg.StopChan():
			return
		default:
		}

		c.managedLockContract(contract.ID)
		c.managedNewContract(HOST, COST, contractsEndHeight) // TODO: Select host and cost.
		c.managedUnlockContract(contract.ID)

		numGoodContracts++
	}
}
