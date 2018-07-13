package contractor

// contracts.go handles forming and renewing contracts for the contractor. This
// includes deciding when new contracts need to be formed, when contracts need
// to be renewed, and if contracts need to be blacklisted.

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/proto"
	"github.com/NebulousLabs/Sia/types"
)

var (
	// ErrInsufficientAllowance indicates that the renter's allowance is less
	// than the amount necessary to store at least one sector
	ErrInsufficientAllowance = errors.New("allowance is not large enough to cover fees of contract creation")
	errTooExpensive          = errors.New("host price was too high")
)

// contractEndHeight returns the height at which the Contractor's contracts
// end. If there are no contracts, it returns zero.
func (c *Contractor) contractEndHeight() types.BlockHeight {
	return c.currentPeriod + c.allowance.Period
}

// managedContractUtility returns the ContractUtility for a contract with a given id.
func (c *Contractor) managedContractUtility(id types.FileContractID) (modules.ContractUtility, bool) {
	rc, exists := c.staticContracts.View(id)
	if !exists {
		return modules.ContractUtility{}, false
	}
	return rc.Utility, true
}

// managedInterruptContractMaintenance will issue an interrupt signal to any
// running maintenance, stopping that maintenance. If there are multiple threads
// running maintenance, they will all be stopped.
func (c *Contractor) managedInterruptContractMaintenance() {
	// Spin up a thread to grab the maintenance lock. Signal that the lock was
	// acquired after the lock is acquired.
	gotLock := make(chan struct{})
	go func() {
		c.maintenanceLock.Lock()
		close(gotLock)
		c.maintenanceLock.Unlock()
	}()

	// There may be multiple threads contending for the maintenance lock. Issue
	// interrupts repeatedly until we get a signal that the maintenance lock has
	// been acquired.
	for {
		select {
		case <-gotLock:
			return
		case c.interruptMaintenance <- struct{}{}:
		}
	}
}

// managedMarkContractsUtility checks every active contract in the contractor and
// figures out whether the contract is useful for uploading, and whether the
// contract should be renewed.
func (c *Contractor) managedMarkContractsUtility() error {
	// Pull a new set of hosts from the hostdb that could be used as a new set
	// to match the allowance. The lowest scoring host of these new hosts will
	// be used as a baseline for determining whether our existing contracts are
	// worthwhile.
	c.mu.RLock()
	hostCount := int(c.allowance.Hosts)
	c.mu.RUnlock()
	hosts, err := c.hdb.RandomHosts(hostCount+randomHostsBufferForScore, nil)
	if err != nil {
		return err
	}

	// Find the minimum score that a host is allowed to have to be considered
	// good for upload.
	var minScore types.Currency
	if len(hosts) > 0 {
		lowestScore := c.hdb.ScoreBreakdown(hosts[0]).Score
		for i := 1; i < len(hosts); i++ {
			score := c.hdb.ScoreBreakdown(hosts[i]).Score
			if score.Cmp(lowestScore) < 0 {
				lowestScore = score
			}
		}
		// Set the minimum acceptable score to a factor of the lowest score.
		minScore = lowestScore.Div(scoreLeeway)
	}

	// Update utility fields for each contract.
	for _, contract := range c.staticContracts.ViewAll() {
		utility := func() (u modules.ContractUtility) {
			// Record current utility of the contract
			u.GoodForRenew = contract.Utility.GoodForRenew
			u.GoodForUpload = contract.Utility.GoodForUpload
			u.Locked = contract.Utility.Locked

			// Start the contract in good standing if the utility wasn't
			// locked.
			if !u.Locked {
				u.GoodForUpload = true
				u.GoodForRenew = true
			}

			host, exists := c.hdb.Host(contract.HostPublicKey)
			// Contract has no utility if the host is not in the database.
			if !exists {
				u.GoodForUpload = false
				u.GoodForRenew = false
				return
			}
			// Contract has no utility if the score is poor.
			if !minScore.IsZero() && c.hdb.ScoreBreakdown(host).Score.Cmp(minScore) < 0 {
				u.GoodForUpload = false
				u.GoodForRenew = false
				return
			}
			// Contract has no utility if the host is offline.
			if isOffline(host) {
				u.GoodForUpload = false
				u.GoodForRenew = false
				return
			}
			// Contract should not be used for uploading if the time has come to
			// renew the contract.
			c.mu.RLock()
			blockHeight := c.blockHeight
			renewWindow := c.allowance.RenewWindow
			c.mu.RUnlock()
			if blockHeight+renewWindow >= contract.EndHeight {
				u.GoodForUpload = false
				return
			}
			return
		}()

		// Apply changes.
		err := c.managedUpdateContractUtility(contract.ID, utility)
		if err != nil {
			return err
		}
	}
	return nil
}

// managedNewContract negotiates an initial file contract with the specified
// host, saves it, and returns it.
func (c *Contractor) managedNewContract(host modules.HostDBEntry, contractFunding types.Currency, endHeight types.BlockHeight) (modules.RenterContract, error) {
	// reject hosts that are too expensive
	if host.StoragePrice.Cmp(maxStoragePrice) > 0 {
		return modules.RenterContract{}, errTooExpensive
	}
	// cap host.MaxCollateral
	if host.MaxCollateral.Cmp(maxCollateral) > 0 {
		host.MaxCollateral = maxCollateral
	}

	// get an address to use for negotiation
	uc, err := c.wallet.NextAddress()
	if err != nil {
		return modules.RenterContract{}, err
	}

	// create contract params
	c.mu.RLock()
	params := proto.ContractParams{
		Host:          host,
		Funding:       contractFunding,
		StartHeight:   c.blockHeight,
		EndHeight:     endHeight,
		RefundAddress: uc.UnlockHash(),
	}
	c.mu.RUnlock()

	// create transaction builder
	txnBuilder, err := c.wallet.StartTransaction()
	if err != nil {
		return modules.RenterContract{}, err
	}

	contract, err := c.staticContracts.FormContract(params, txnBuilder, c.tpool, c.hdb, c.tg.StopChan())
	if err != nil {
		txnBuilder.Drop()
		return modules.RenterContract{}, err
	}

	// Add a mapping from the contract's id to the public key of the host.
	c.mu.Lock()
	c.contractIDToPubKey[contract.ID] = contract.HostPublicKey
	_, exists := c.pubKeysToContractID[string(contract.HostPublicKey.Key)]
	if exists {
		c.mu.Unlock()
		txnBuilder.Drop()
		return modules.RenterContract{}, fmt.Errorf("We already have a contract with host %v", contract.HostPublicKey)
	}
	c.pubKeysToContractID[string(contract.HostPublicKey.Key)] = contract.ID
	c.mu.Unlock()

	contractValue := contract.RenterFunds
	c.log.Printf("Formed contract %v with %v for %v", contract.ID, host.NetAddress, contractValue.HumanString())
	return contract, nil
}

// managedPrunePubkeyMap will delete any pubkeys in the pubKeysToContractID map
// that no longer map to an active contract.
func (c *Contractor) managedPrunePubkeyMap() {
	allContracts := c.staticContracts.ViewAll()
	pks := make(map[string]struct{})
	for _, c := range allContracts {
		pks[string(c.HostPublicKey.Key)] = struct{}{}
	}
	c.mu.Lock()
	for pk := range c.pubKeysToContractID {
		if _, exists := pks[pk]; !exists {
			delete(c.pubKeysToContractID, pk)
		}
	}
	c.mu.Unlock()
}

// managedRenew negotiates a new contract for data already stored with a host.
// It returns the new contract. This is a blocking call that performs network
// I/O.
func (c *Contractor) managedRenew(sc *proto.SafeContract, contractFunding types.Currency, newEndHeight types.BlockHeight) (modules.RenterContract, error) {
	// For convenience
	contract := sc.Metadata()
	// Sanity check - should not be renewing a bad contract.
	utility, ok := c.managedContractUtility(contract.ID)
	if !ok || !utility.GoodForRenew {
		c.log.Critical(fmt.Sprintf("Renewing a contract that has been marked as !GoodForRenew %v/%v",
			ok, utility.GoodForRenew))
	}

	// Fetch the host associated with this contract.
	host, ok := c.hdb.Host(contract.HostPublicKey)
	if !ok {
		return modules.RenterContract{}, errors.New("no record of that host")
	} else if host.StoragePrice.Cmp(maxStoragePrice) > 0 {
		return modules.RenterContract{}, errTooExpensive
	}
	// cap host.MaxCollateral
	if host.MaxCollateral.Cmp(maxCollateral) > 0 {
		host.MaxCollateral = maxCollateral
	}

	// get an address to use for negotiation
	uc, err := c.wallet.NextAddress()
	if err != nil {
		return modules.RenterContract{}, err
	}

	// create contract params
	c.mu.RLock()
	params := proto.ContractParams{
		Host:          host,
		Funding:       contractFunding,
		StartHeight:   c.blockHeight,
		EndHeight:     newEndHeight,
		RefundAddress: uc.UnlockHash(),
	}
	c.mu.RUnlock()

	// execute negotiation protocol
	txnBuilder, err := c.wallet.StartTransaction()
	if err != nil {
		return modules.RenterContract{}, err
	}
	newContract, err := c.staticContracts.Renew(sc, params, txnBuilder, c.tpool, c.hdb, c.tg.StopChan())
	if err != nil {
		txnBuilder.Drop() // return unused outputs to wallet
		return modules.RenterContract{}, err
	}

	// Add a mapping from the contract's id to the public key of the host. This
	// will destroy the previous mapping from pubKey to contract id but other
	// modules are only interested in the most recent contract anyway.
	c.mu.Lock()
	c.contractIDToPubKey[newContract.ID] = newContract.HostPublicKey
	c.pubKeysToContractID[string(newContract.HostPublicKey.Key)] = newContract.ID
	c.mu.Unlock()

	return newContract, nil
}

// threadedContractMaintenance checks the set of contracts that the contractor
// has against the allownace, renewing any contracts that need to be renewed,
// dropping contracts which are no longer worthwhile, and adding contracts if
// there are not enough.
//
// Between each network call, the thread checks whether a maintenance interrupt
// signal is being sent. If so, maintenance returns, yielding to whatever thread
// issued the interrupt.
func (c *Contractor) threadedContractMaintenance() {
	// Threading protection.
	err := c.tg.Add()
	if err != nil {
		return
	}
	defer c.tg.Done()

	// Archive contracts that need to be archived before doing additional
	// maintenance, and then prune the pubkey map.
	c.managedArchiveContracts()
	c.managedPrunePubkeyMap()

	// Nothing to do if there are no hosts.
	c.mu.RLock()
	wantedHosts := c.allowance.Hosts
	c.mu.RUnlock()
	if wantedHosts <= 0 {
		return
	}

	// Only one instance of this thread should be running at a time. Under
	// normal conditions, fine to return early if another thread is already
	// doing maintenance. The next block will trigger another round. Under
	// testing, control is insufficient if the maintenance loop isn't guaranteed
	// to run.
	if build.Release == "testing" {
		c.maintenanceLock.Lock()
	} else if !c.maintenanceLock.TryLock() {
		return
	}
	defer c.maintenanceLock.Unlock()

	// Update the utility fields for this contract based on the most recent
	// hostdb.
	if err := c.managedMarkContractsUtility(); err != nil {
		c.log.Println("WARNING: wasn't able to mark contracts", err)
		return
	}

	// Figure out which contracts need to be renewed, and while we have the
	// lock, figure out the end height for the new contracts and also the amount
	// to spend on each contract.
	//
	// refreshSet is used to mark contracts that need to be refreshed because
	// they have run out of money. The refreshSet indicates how much currency
	// was used previously in the contract line, and is used to figure out how
	// much additional money to add in the refreshed contract.
	//
	// The actions inside this RLock are complex enough to merit wrapping them
	// in a function where we can defer the unlock.
	type renewal struct {
		id     types.FileContractID
		amount types.Currency
	}
	var endHeight types.BlockHeight
	var fundsAvailable types.Currency
	var renewSet []renewal

	c.mu.RLock()
	currentPeriod := c.currentPeriod
	allowance := c.allowance
	blockHeight := c.blockHeight
	c.mu.RUnlock()

	// Grab the end height that should be used for the contracts created
	// in the current period.
	endHeight = currentPeriod + allowance.Period

	// Determine how many funds have been used already in this billing cycle,
	// and how many funds are remaining. We have to calculate these numbers
	// separately to avoid underflow, and then re-join them later to get the
	// full picture for how many funds are available.
	var fundsUsed types.Currency
	for _, contract := range c.staticContracts.ViewAll() {
		// Calculate the cost of the contract line.
		contractLineCost := contract.TotalCost

		// Check if the contract is expiring. The funds in the contract are
		// handled differently based on this information.
		if blockHeight+allowance.RenewWindow >= contract.EndHeight {
			// The contract is expiring. Some of the funds are locked down to
			// renew the contract, and then the remaining funds can be allocated
			// to 'availableFunds'.
			fundsUsed = fundsUsed.Add(contractLineCost).Sub(contract.RenterFunds)
			fundsAvailable = fundsAvailable.Add(contract.RenterFunds)
		} else {
			// The contract is not expiring. None of the funds in the contract
			// are available to renew or form contracts.
			fundsUsed = fundsUsed.Add(contractLineCost)
		}
	}

	// Add any unspent funds from the allowance to the available funds. If the
	// allowance has been decreased, it's possible that we actually need to
	// reduce the number of funds available to compensate.
	if fundsAvailable.Add(allowance.Funds).Cmp(fundsUsed) > 0 {
		fundsAvailable = fundsAvailable.Add(allowance.Funds).Sub(fundsUsed)
	} else {
		// Figure out how much we need to remove from fundsAvailable to clear
		// the allowance.
		overspend := fundsUsed.Sub(allowance.Funds).Sub(fundsAvailable)
		if fundsAvailable.Cmp(overspend) > 0 {
			// We still have some funds available.
			fundsAvailable = fundsAvailable.Sub(overspend)
		} else {
			// The overspend exceeds the available funds, set available funds to
			// zero.
			fundsAvailable = types.ZeroCurrency
		}
	}

	// Iterate through the contracts again, figuring out which contracts to
	// renew and how much extra funds to renew them with.
	for _, contract := range c.staticContracts.ViewAll() {
		utility, ok := c.managedContractUtility(contract.ID)
		if !ok || !utility.GoodForRenew {
			continue
		}
		if blockHeight+allowance.RenewWindow >= contract.EndHeight {
			// This contract needs to be renewed because it is going to expire
			// soon. First step is to calculate how much money should be used in
			// the renewal, based on how much of the contract funds (including
			// previous contracts this billing cycle due to financial resets)
			// were spent throughout this billing cycle.
			//
			// The amount we care about is the total amount that was spent on
			// uploading, downloading, and storage throughout the billing cycle.
			// This is calculated by starting with the total cost and
			// subtracting out all of the fees, and then all of the unused money
			// that was allocated (the RenterFunds).
			//
			// In order to accurately fund contracts based on variable spending,
			// the cost per block is calculated based on the total spent over
			// the length of time that the contract was active before renewal.
			oldContractSpent := contract.TotalCost.Sub(contract.ContractFee).Sub(contract.TxnFee).Sub(contract.SiafundFee).Sub(contract.RenterFunds)
			oldContractLength := blockHeight - contract.StartHeight
			if oldContractLength == 0 {
				oldContractLength = types.BlockHeight(1)
			}
			spentPerBlock := oldContractSpent.Div64(uint64(oldContractLength))
			renewAmount := spentPerBlock.Mul64(uint64(allowance.Period))

			// Get an estimate for how much the fees will cost. Txn Fee
			_, maxTxnFee := c.tpool.FeeEstimation()

			// SiafundFee
			siafundFee := types.Tax(blockHeight, renewAmount)

			// Contract Fee
			host, ok := c.hdb.Host(contract.HostPublicKey)
			if !ok {
				c.log.Println("Could not find contract host in hostdb")
				return
			}

			estimatedFees := host.ContractPrice.Add(maxTxnFee).Add(siafundFee)
			renewAmount = renewAmount.Add(estimatedFees)

			// Determine if there is enough funds available to supplement with a
			// 33% bonus, and if there is, add a 33% bonus.
			moneyBuffer := renewAmount.Div64(3)
			if moneyBuffer.Cmp(fundsAvailable) < 0 {
				renewAmount = renewAmount.Add(moneyBuffer)
				fundsAvailable = fundsAvailable.Sub(moneyBuffer)
			} else {
				c.log.Println("WARN: performing a limited renew due to low allowance")
			}

			// The contract needs to be renewed because it is going to expire
			// soon, and we need to refresh the time.
			renewSet = append(renewSet, renewal{
				id:     contract.ID,
				amount: renewAmount,
			})
		} else {
			// Check if the contract has exhausted its funding and requires
			// premature renewal.
			host, _ := c.hdb.Host(contract.HostPublicKey)

			// Skip this host if its prices are too high.
			// managedMarkContractsUtility should make this redundant, but this
			// is here for extra safety.
			if host.StoragePrice.Cmp(maxStoragePrice) > 0 || host.UploadBandwidthPrice.Cmp(maxUploadPrice) > 0 {
				continue
			}

			blockBytes := types.NewCurrency64(modules.SectorSize * uint64(contract.EndHeight-blockHeight))
			sectorStoragePrice := host.StoragePrice.Mul(blockBytes)
			sectorBandwidthPrice := host.UploadBandwidthPrice.Mul64(modules.SectorSize)
			sectorPrice := sectorStoragePrice.Add(sectorBandwidthPrice)
			percentRemaining, _ := big.NewRat(0, 1).SetFrac(contract.RenterFunds.Big(), contract.TotalCost.Big()).Float64()
			if contract.RenterFunds.Cmp(sectorPrice.Mul64(3)) < 0 || percentRemaining < minContractFundRenewalThreshold {
				// This contract does need to be refreshed. Make sure there are
				// enough funds available to perform the refresh, and then
				// execute.
				oldDuration := blockHeight - contract.StartHeight
				newDuration := endHeight - blockHeight
				spendPerBlock := contract.TotalCost.Div64(uint64(oldDuration))
				refreshAmount := spendPerBlock.Mul64(uint64(newDuration))

				if refreshAmount.Cmp(fundsAvailable) < 0 {
					renewSet = append(renewSet, renewal{
						id:     contract.ID,
						amount: refreshAmount,
					})
				} else {
					c.log.Println("WARN: cannot refresh empty contract due to low allowance.")
				}
			}
		}
	}
	if len(renewSet) != 0 {
		c.log.Printf("renewing %v contracts", len(renewSet))
	}

	// Remove contracts that are not scheduled for renew from firstFailedRenew.
	c.mu.Lock()
	newFirstFailedRenew := make(map[types.FileContractID]types.BlockHeight)
	for _, r := range renewSet {
		if _, exists := c.numFailedRenews[r.id]; exists {
			newFirstFailedRenew[r.id] = c.numFailedRenews[r.id]
		}
	}
	c.numFailedRenews = newFirstFailedRenew
	c.mu.Unlock()

	// Loop through the contracts and renew them one-by-one.
	for _, renewal := range renewSet {
		// Pull the variables out of the renewal.
		id := renewal.id
		amount := renewal.amount

		// Renew one contract.
		func() {
			// Mark the contract as being renewed, and defer logic to unmark it
			// once renewing is complete.
			c.mu.Lock()
			c.renewing[id] = true
			c.mu.Unlock()
			defer func() {
				c.mu.Lock()
				delete(c.renewing, id)
				c.mu.Unlock()
			}()

			// Wait for any active editors and downloaders to finish for this
			// contract, and then grab the latest revision.
			c.mu.RLock()
			e, eok := c.editors[id]
			d, dok := c.downloaders[id]
			c.mu.RUnlock()
			if eok {
				e.invalidate()
			}
			if dok {
				d.invalidate()
			}

			// Fetch the contract that we are renewing.
			oldContract, exists := c.staticContracts.Acquire(id)
			if !exists {
				return
			}
			// Return the contract if it's not useful for renewing.
			oldUtility, ok := c.managedContractUtility(id)
			if !ok || !oldUtility.GoodForRenew {
				c.log.Printf("Contract %v slated for renew is marked not good for renew %v/%v",
					id, ok, oldUtility.GoodForRenew)
				c.staticContracts.Return(oldContract)
				return
			}

			// Calculate endHeight for renewed contracts
			endHeight = currentPeriod + allowance.Period

			// Perform the actual renew. If the renew fails, return the
			// contract. If the renew fails we check how often it has failed
			// before. Once it has failed for a certain number of blocks in a
			// row and reached its second half of the renew window, we give up
			// on renewing it and set goodForRenew to false.
			newContract, errRenew := c.managedRenew(oldContract, amount, endHeight)
			if errRenew != nil {
				// Increment the number of failed renews for the contract if it
				// was the host's fault.
				if modules.IsHostsFault(errRenew) {
					c.mu.Lock()
					c.numFailedRenews[oldContract.Metadata().ID]++
					c.mu.Unlock()
				}

				// Check if contract has to be replaced.
				md := oldContract.Metadata()
				c.mu.RLock()
				numRenews, failedBefore := c.numFailedRenews[md.ID]
				c.mu.RUnlock()
				secondHalfOfWindow := blockHeight+allowance.RenewWindow/2 >= md.EndHeight
				replace := numRenews >= consecutiveRenewalsBeforeReplacement
				if failedBefore && secondHalfOfWindow && replace {
					oldUtility.GoodForRenew = false
					oldUtility.GoodForUpload = false
					oldUtility.Locked = true
					err := oldContract.UpdateUtility(oldUtility)
					if err != nil {
						c.log.Println("WARN: failed to mark contract as !goodForRenew:", err)
					}
					c.log.Printf("WARN: failed to renew %v, marked as bad: %v\n",
						oldContract.Metadata().HostPublicKey, errRenew)
					c.staticContracts.Return(oldContract)
					return
				}

				// Seems like it doesn't have to be replaced yet. Log the
				// failure and number of renews that have failed so far.
				c.log.Printf("WARN: failed to renew contract %v [%v]: %v\n",
					oldContract.Metadata().HostPublicKey, numRenews, errRenew)
				c.staticContracts.Return(oldContract)
				return
			}
			c.log.Printf("Renewed contract %v\n", id)

			// Update the utility values for the new contract, and for the old
			// contract.
			newUtility := modules.ContractUtility{
				GoodForUpload: true,
				GoodForRenew:  true,
			}
			if err := c.managedUpdateContractUtility(newContract.ID, newUtility); err != nil {
				c.log.Println("Failed to update the contract utilities", err)
				return
			}
			oldUtility.GoodForRenew = false
			oldUtility.GoodForUpload = false
			if err := oldContract.UpdateUtility(oldUtility); err != nil {
				c.log.Println("Failed to update the contract utilities", err)
				return
			}

			// Lock the contractor as we update it to use the new contract
			// instead of the old contract.
			c.mu.Lock()
			defer c.mu.Unlock()
			// Delete the old contract.
			c.staticContracts.Delete(oldContract)
			// Store the contract in the record of historic contracts.
			c.oldContracts[id] = oldContract.Metadata()
			// Save the contractor.
			err = c.saveSync()
			if err != nil {
				c.log.Println("Failed to save the contractor after creating a new contract.")
			}
		}()

		// Soft sleep for a minute to allow all of the transactions to propagate
		// the network.
		select {
		case <-c.tg.StopChan():
			return
		case <-c.interruptMaintenance:
			return
		default:
		}
	}

	// Quit in the event of shutdown.
	select {
	case <-c.tg.StopChan():
		return
	case <-c.interruptMaintenance:
		return
	default:
	}

	// Count the number of contracts which are good for uploading, and then make
	// more as needed to fill the gap.
	uploadContracts := 0
	for _, id := range c.staticContracts.IDs() {
		if cu, ok := c.managedContractUtility(id); ok && cu.GoodForUpload {
			uploadContracts++
		}
	}
	c.mu.RLock()
	neededContracts := int(c.allowance.Hosts) - uploadContracts
	c.mu.RUnlock()
	if neededContracts <= 0 {
		return
	}

	// Assemble an exclusion list that includes all of the hosts that we already
	// have contracts with, then select a new batch of hosts to attempt contract
	// formation with.
	c.mu.RLock()
	var exclude []types.SiaPublicKey
	for _, contract := range c.staticContracts.ViewAll() {
		exclude = append(exclude, contract.HostPublicKey)
	}
	initialContractFunds := c.allowance.Funds.Div64(c.allowance.Hosts).Div64(3)
	c.mu.RUnlock()
	hosts, err := c.hdb.RandomHosts(neededContracts*2+randomHostsBufferForScore, exclude)
	if err != nil {
		c.log.Println("WARN: not forming new contracts:", err)
		return
	}

	// Form contracts with the hosts one at a time, until we have enough
	// contracts.
	for _, host := range hosts {
		// Determine if we have enough money to form a new contract.
		if fundsAvailable.Cmp(initialContractFunds) < 0 {
			c.log.Println("WARN: need to form new contracts, but unable to because of a low allowance")
			break
		}

		// Attempt forming a contract with this host.
		newContract, err := c.managedNewContract(host, initialContractFunds, endHeight)
		if err != nil {
			c.log.Printf("Attempted to form a contract with %v, but negotiation failed: %v\n", host.NetAddress, err)
			continue
		}

		// Add this contract to the contractor and save.
		err = c.managedUpdateContractUtility(newContract.ID, modules.ContractUtility{
			GoodForUpload: true,
			GoodForRenew:  true,
		})
		if err != nil {
			c.log.Println("Failed to update the contract utilities", err)
			return
		}
		c.mu.Lock()
		err = c.saveSync()
		c.mu.Unlock()
		if err != nil {
			c.log.Println("Unable to save the contractor:", err)
		}

		// Quit the loop if we've replaced all needed contracts.
		neededContracts--
		if neededContracts <= 0 {
			break
		}

		// Soft sleep before making the next contract.
		select {
		case <-c.tg.StopChan():
			return
		case <-c.interruptMaintenance:
			return
		default:
		}
	}
}

// managedUpdateContractUtility is a helper function that acquires a contract, updates
// its ContractUtility and returns the contract again.
func (c *Contractor) managedUpdateContractUtility(id types.FileContractID, utility modules.ContractUtility) error {
	safeContract, ok := c.staticContracts.Acquire(id)
	if !ok {
		return errors.New("failed to acquire contract for update")
	}
	defer c.staticContracts.Return(safeContract)
	return safeContract.UpdateUtility(utility)
}
