package contractor

import (
	"sort"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

// callateralFromCost calculates the appropriate collateral to expect from the
// host given the contractor's preferences, the host's preferences, and the
// amount of funds being thrown into the contract.
func (c *Contractor) collateralFromCost(funds types.Currency, host modules.HostDBEntry) types.Currency {
	// Calculate the desired collateral.
	collateral := funds.Mul(host.Collateral).Div(host.StoragePrice)
	// Don't exceed the host's maximum collateral.
	if collateral.Cmp(host.MaxCollateral) > 0 {
		collateral = host.MaxCollateral
	}
	return collateral
}

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

// managedMarkBadContracts will go through the contractors set of contracts and mark
// any of the contracts which are no longer performing well.
func (c *Contractor) managedMarkBadContracts() {
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

	c.mu.Lock()
	defer c.mu.Unlock()

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
			c.allowance.Funds = c.allowance.Funds.Sub(contract.TotalCost)
			c.contracts[contract.ID] = contract
			continue
		}

		// Check whether the contract still has an acceptable score in the
		// hostdb.
		host, exists := c.hdb.Host(contract.HostPublicKey)
		if !exists {
			contract.InGoodStanding = false
			c.allowance.Funds = c.allowance.Funds.Sub(contract.TotalCost)
			c.contracts[contract.ID] = contract
			continue
		}
		if c.hdb.ScoreBreakdown(host).Score.Cmp(lowestScore) < 0 {
			// Host's score is unaccepably low.
			contract.InGoodStanding = false
			c.allowance.Funds = c.allowance.Funds.Sub(contract.TotalCost)
			c.contracts[contract.ID] = contract
			continue
		}

		// Determine whether the host has enough storage space remaining to be
		// useful for uploading. Note that the amount of storage a host has
		// remaining is dynamic, and so the host may become useful for uploading
		// in the near future.
		if host.RemainingStorage < storageRemainingThreshold {
			contract.UsefulForUpload = false
			// If the renter is not storing enough data on this host, consider
			// the host to be too expensive, and mark the host for replacement.
			// For the sake of simplicity, this value is being set to a
			// constant, but eventually it should be determined based on things
			// like the storage price, the contract price, and the other hosts.
			if contract.LastRevision.NewFileSize < 50e9 {
				contract.InGoodStanding = false
				c.allowance.Funds = c.allowance.Funds.Sub(contract.TotalCost)
			}
		} else {
			contract.UsefulForUpload = true
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

	// The repair contracts loop will not run if the consensus set is not
	// synced.
	if !c.cs.Synced() {
		return
	}

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

	// TODO: If the allowance changes mid-function, abort and restart.

	// Determine whether the current period has advanced to the next period.
	c.mu.Lock()
	if c.currentPeriod+c.allowance.Period < c.blockHeight+c.allowance.RenewWindow {
		c.currentPeriod = c.blockHeight
	}
	c.mu.Unlock()

	// Iterate through the set of contracts that have expired and move them to
	// the archive.
	c.mu.Lock()
	var expiredContracts []modules.RenterContract
	for _, contract := range c.contracts {
		if c.blockHeight > contract.EndHeight() {
			expiredContracts = append(expiredContracts, contract)
		}
	}
	for _, contract := range expiredContracts {
		// Archive the contract.
		c.oldContracts[contract.ID] = contract
		// Delete the contract.
		delete(c.contracts, contract.ID)
		// Delete the cached revision.
		delete(c.cachedRevisions, contract.ID)
		// Update the allowance to account for disappeared spending.
		c.allowance.Funds = c.allowance.Funds.Sub(contract.TotalCost)
		// Save the changes.
		err = c.saveSync()
		if err != nil {
			c.log.Println("Unable to save the contractor after renewing a contract:", err)
		}
	}
	c.mu.Unlock()

	// Reveiw the set of contracts held by the contractor, and mark any
	// contracts whose hosts have fallen out of favor.
	c.managedMarkBadContracts()

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
			_, exists := c.hdb.Host(contract.HostPublicKey)
			if !exists {
				continue // This contract is with a host that doesn't exist.
			}

			// Check that the contract is not empty.
			empty := contract.RenterFunds().Cmp(lowContractBalance) <= 0
			empty = empty && contract.UsefulForUpload
			// Check that the contract is not expiring soon.
			expiring := c.currentPeriod+c.allowance.Period-c.allowance.RenewWindow >= contract.EndHeight()
			if empty || expiring {
				needsRenew = append(needsRenew, contract)
			}

			// Host does not count as a 'good' contract if it is not useful for
			// upload.
			if !contract.UsefulForUpload {
				continue
			}
			// Contract counts as good regardless of whether it needs to be
			// renewed.
			numGoodContracts++
		}
	}()

	// Get the height that the contracts should be formed at.
	c.mu.Lock()
	contractsEndHeight := c.currentPeriod + c.allowance.Period
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
			continue // Can't renew without host.
		}

		// Figure out how much money to give the host.
		var newCost types.Currency
		empty := contract.RenterFunds().Cmp(lowContractBalance) <= 0
		if empty {
			// Contract is being renewed because it ran out of money. Double the
			// amount of money that's allocated to the contract.
			prevCost := contract.TotalCost.Sub(contract.ContractFee).Sub(contract.TxnFee).Sub(contract.SiafundFee)
			newCost = prevCost.Mul64(2)
		} else {
			prevBase := contract.TotalCost.Sub(contract.ContractFee).Sub(contract.TxnFee).Sub(contract.SiafundFee)
			// The amount of money that the contract started with should not be
			// less than the amount of money remaining in the contract, but
			// double check just to be sure.
			if prevBase.Cmp(contract.RenterFunds()) < 0 {
				build.Critical("A contracts base funds is smaller than it's available funds:", prevBase, contract.RenterFunds())
			}
			prevCost := prevBase.Sub(contract.RenterFunds())
			newCost = prevCost.Mul64(3).Div64(2) // Set the renterFunds to 50% more than the amount of money spent in the last cylce.
		}

		// Verify that the new cost is at least enough to cover all the existing
		// data and some extra.
		timeExtension := uint64(contractsEndHeight - contract.LastRevision.NewWindowEnd) // TODO: May need to add host.WindowSize too.
		// Determine how much is going to be spend immediately on existing
		// storage.
		baseCost := host.StoragePrice.Mul64(contract.LastRevision.NewFileSize).Mul64(timeExtension)
		// Make sure that the renterFunds are at least 50% greater than the base
		// cost.
		minFunds := baseCost.Mul64(3).Div64(2)
		if minFunds.Cmp(newCost) < 0 {
			newCost = minFunds
		}
		// Make sure there is at least a minimum number of coins in the
		// contract.
		if newCost.Cmp(minContractFunds) < 0 {
			newCost = minContractFunds
		}

		// Calculate the desired collateral.
		collateral := c.collateralFromCost(newCost, host)

		c.managedLockContract(contract.ID)
		c.managedRenewContract(contract, host, newCost, collateral, contractsEndHeight)
		c.managedUnlockContract(contract.ID)

		// Soft sleep between contract formation.
		select {
		case <-c.tg.StopChan():
			return
		case <-time.After(contractFormationInterval):
		}
	}

	// Determine how many new contracts we want to form. Also grab the list of
	// hosts that we already have, so that we do not form any duplicate
	// contracts.
	c.mu.Lock()
	existingHosts := make([]types.SiaPublicKey, 0, len(c.contracts))
	wantedHosts := c.allowance.Hosts
	for _, contract := range c.contracts {
		existingHosts = append(existingHosts, contract.HostPublicKey)
	}
	c.mu.Unlock()

	// Select a bunch of new hosts from the database.
	selectionSize := int(wantedHosts*3 + 10)                 // Fine to have an abundance of hosts.
	hosts := c.hdb.RandomHosts(selectionSize, existingHosts) // Fine to have an abundance of hosts.

	// Filter any hosts that do not have enough storage remaining.
	i := 0
	for {
		if i >= len(hosts) {
			break
		}
		if hosts[i].RemainingStorage < storageRemainingThreshold {
			hosts = append(hosts[:i], hosts[i+1:]...)
		} else {
			i++
		}

	}

	// Form contracts with the hosts until we have the desired number of total
	// contracts.
	i = 0
	for numGoodContracts < wantedHosts && i < len(hosts) {
		// Exit if stop has been called.
		select {
		case <-c.tg.StopChan():
			return
		default:
		}

		// Try to form a contract with this host. Set the price to the min funds
		// plus twice the contract price. Future interactions will guide the
		// price to something more reasonalbe.
		contractFunds := minContractFunds.Add(hosts[i].ContractPrice.Mul64(2))

		contractCollateral := c.collateralFromCost(contractFunds, hosts[i])
		err = c.managedNewContract(hosts[i], contractFunds, contractCollateral, contractsEndHeight)
		if err != nil {
			c.log.Debugln("Unable to form contract with host:", err)
		} else {
			numGoodContracts++
		}
		i++

		// Soft sleep between contract formation.
		select {
		case <-c.tg.StopChan():
			return
		case <-time.After(contractFormationInterval):
		}
	}
}
