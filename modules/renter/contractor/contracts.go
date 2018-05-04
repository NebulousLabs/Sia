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

// readlockContractUtility returns the ContractUtility for a contract with a given id.
func (c *Contractor) readlockContractUtility(id types.FileContractID) (modules.ContractUtility, bool) {
	rc, exists := c.contracts.View(c.readlockResolveID(id))
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
// figures out whether the contract is useful for uploading, and whehter the
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
	for _, contract := range c.contracts.ViewAll() {
		utility := func() (u modules.ContractUtility) {
			// Start the contract in good standing.
			u.GoodForUpload = true
			u.GoodForRenew = true

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
			// Contract has no utility if renew has already completed. (grab some
			// extra values while we have the mutex)
			c.mu.RLock()
			blockHeight := c.blockHeight
			renewWindow := c.allowance.RenewWindow
			_, renewedPreviously := c.renewedIDs[contract.ID]
			c.mu.RUnlock()
			if renewedPreviously {
				u.GoodForUpload = false
				u.GoodForRenew = false
				return
			}

			// Contract should not be used for uploading if the time has come to
			// renew the contract.
			if blockHeight+renewWindow >= contract.EndHeight {
				u.GoodForUpload = false
				return
			}
			return
		}()

		// Apply changes.
		c.mu.Lock()
		err := c.updateContractUtility(contract.ID, utility)
		c.mu.Unlock()
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
	txnBuilder := c.wallet.StartTransaction()

	contract, err := c.contracts.FormContract(params, txnBuilder, c.tpool, c.hdb, c.tg.StopChan())
	if err != nil {
		txnBuilder.Drop()
		return modules.RenterContract{}, err
	}

	contractValue := contract.RenterFunds
	c.log.Printf("Formed contract %v with %v for %v", contract.ID, host.NetAddress, contractValue.HumanString())
	return contract, nil
}

// managedRenew negotiates a new contract for data already stored with a host.
// It returns the new contract. This is a blocking call that performs network
// I/O.
func (c *Contractor) managedRenew(sc *proto.SafeContract, contractFunding types.Currency, newEndHeight types.BlockHeight) (modules.RenterContract, error) {
	// For convenience
	contract := sc.Metadata()
	// Sanity check - should not be renewing a bad contract.
	c.mu.RLock()
	utility, ok := c.readlockContractUtility(contract.ID)
	c.mu.RUnlock()
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
	txnBuilder := c.wallet.StartTransaction()
	newContract, err := c.contracts.Renew(sc, params, txnBuilder, c.tpool, c.hdb, c.tg.StopChan())
	if err != nil {
		txnBuilder.Drop() // return unused outputs to wallet
		return modules.RenterContract{}, err
	}

	return newContract, nil
}

// threadedContractMaintenance checks the set of contracts that the contractor
// has against the allownace, renewing any contracts that need to be renewed,
// dropping contracts which are no longer worthwhile, and adding contracts if
// there are not enough.
//
// Between each network call, the thread checks whether a maintenance iterrupt
// signal is being sent. If so, maintannce returns, yielding to whatever thread
// issued the interrupt.
func (c *Contractor) threadedContractMaintenance() {
	// Threading protection.
	err := c.tg.Add()
	if err != nil {
		return
	}
	defer c.tg.Done()

	// Archive contracts that need to be archived before doing additional maintenance.
	c.managedArchiveContracts()

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
	refreshSet := make(map[types.FileContractID]struct{})
	func() {
		c.mu.RLock()
		defer c.mu.RUnlock()

		// Grab the end height that should be used for the contracts.
		endHeight = c.currentPeriod + c.allowance.Period

		// Determine how many funds have been used already in this billing
		// cycle, and how many funds are remaining. We have to calculate these
		// numbers separately to avoid underflow, and then re-join them later to
		// get the full picture for how many funds are available.
		var fundsUsed types.Currency
		for _, contract := range c.contracts.ViewAll() {
			// Calculate the cost of the contract line.
			contractLineCost := contract.TotalCost
			// TODO: add previous contracts here

			// Check if the contract is expiring. The funds in the contract are
			// handled differently based on this information.
			if c.blockHeight+c.allowance.RenewWindow >= contract.EndHeight {
				// The contract is expiring. Some of the funds are locked down
				// to renew the contract, and then the remaining funds can be
				// allocated to 'availableFunds'.
				fundsUsed = fundsUsed.Add(contractLineCost).Sub(contract.RenterFunds)
				fundsAvailable = fundsAvailable.Add(contract.RenterFunds)
			} else {
				// The contract is not expiring. None of the funds in the
				// contract are available to renew or form contracts.
				fundsUsed = fundsUsed.Add(contractLineCost)
			}
		}

		// Add any unspent funds from the allowance to the available funds. If
		// the allowance has been decreased, it's possible that we actually need
		// to reduce the number of funds available to compensate.
		if fundsAvailable.Add(c.allowance.Funds).Cmp(fundsUsed) > 0 {
			fundsAvailable = fundsAvailable.Add(c.allowance.Funds).Sub(fundsUsed)
		} else {
			// Figure out how much we need to remove from fundsAvailable to
			// clear the allowance.
			overspend := fundsUsed.Sub(c.allowance.Funds).Sub(fundsAvailable)
			if fundsAvailable.Cmp(overspend) > 0 {
				// We still have some funds available.
				fundsAvailable = fundsAvailable.Sub(overspend)
			} else {
				// The overspend exceeds the available funds, set available
				// funds to zero.
				fundsAvailable = types.ZeroCurrency
			}
		}

		// Iterate through the contracts again, figuring out which contracts to
		// renew and how much extra funds to renew them with.
		for _, contract := range c.contracts.ViewAll() {
			utility, ok := c.readlockContractUtility(contract.ID)
			if !ok || !utility.GoodForRenew {
				continue
			}
			if c.blockHeight+c.allowance.RenewWindow >= contract.EndHeight {
				// This contract needs to be renewed because it is going to
				// expire soon. First step is to calculate how much money should
				// be used in the renewal, based on how much of the contract
				// funds (including previous contracts this billing cycle due to
				// financial resets) were spent throughout this billing cycle.
				//
				// The amount we care about is the total amount that was spent
				// on uploading, downloading, and storage throughout the billing
				// cycle. This is calculated by starting with the total cost and
				// subtracting out all of the fees, and then all of the unused
				// money that was allocated (the RenterFunds).
				renewAmount := contract.TotalCost.Sub(contract.ContractFee).Sub(contract.TxnFee).Sub(contract.SiafundFee).Sub(contract.RenterFunds)
				// TODO: add previous contracts here

				// Get an estimate for how much the fees will cost.
				//
				// TODO: Look up this host in the hostdb to figure out what the
				// actual fees will be.
				estimatedFees := contract.ContractFee.Add(contract.TxnFee).Add(contract.SiafundFee)
				renewAmount = renewAmount.Add(estimatedFees)

				// Determine if there is enough funds available to suppliement
				// with a 33% bonus, and if there is, add a 33% bonus.
				moneyBuffer := renewAmount.Div64(3)
				if moneyBuffer.Cmp(fundsAvailable) < 0 {
					renewAmount = renewAmount.Add(moneyBuffer)
					fundsAvailable = fundsAvailable.Sub(moneyBuffer)
				} else {
					c.log.Println("WARN: performing a limited renew due to low allowance")
				}

				// The contract needs to be renewed because it is going to
				// expire soon, and we need to refresh the time.
				renewSet = append(renewSet, renewal{
					id:     contract.ID,
					amount: renewAmount,
				})
			} else {
				// Check if the contract has exhausted its funding and requires
				// premature renewal.
				c.mu.RUnlock()
				host, _ := c.hdb.Host(contract.HostPublicKey)
				c.mu.RLock()

				// Skip this host if its prices are too high.
				// managedMarkContractsUtility should make this redundant, but
				// this is here for extra safety.
				if host.StoragePrice.Cmp(maxStoragePrice) > 0 || host.UploadBandwidthPrice.Cmp(maxUploadPrice) > 0 {
					continue
				}

				blockBytes := types.NewCurrency64(modules.SectorSize * uint64(contract.EndHeight-c.blockHeight))
				sectorStoragePrice := host.StoragePrice.Mul(blockBytes)
				sectorBandwidthPrice := host.UploadBandwidthPrice.Mul64(modules.SectorSize)
				sectorPrice := sectorStoragePrice.Add(sectorBandwidthPrice)
				percentRemaining, _ := big.NewRat(0, 1).SetFrac(contract.RenterFunds.Big(), contract.TotalCost.Big()).Float64()
				if contract.RenterFunds.Cmp(sectorPrice.Mul64(3)) < 0 || percentRemaining < minContractFundRenewalThreshold {
					// This contract does need to be refreshed. Make sure there
					// are enough funds available to perform the refresh, and
					// then execute.
					refreshAmount := contract.TotalCost.Mul64(2)
					if refreshAmount.Cmp(fundsAvailable) < 0 {
						refreshSet[contract.ID] = struct{}{}
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
	}()
	if len(renewSet) != 0 {
		c.log.Printf("renewing %v contracts", len(renewSet))
	}

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
			oldContract, exists := c.contracts.Acquire(id)
			if !exists {
				return
			}
			// Return the contract if it's not useful for renewing.
			c.mu.RLock()
			oldUtility, ok := c.readlockContractUtility(id)
			c.mu.RUnlock()
			if !ok || !oldUtility.GoodForRenew {
				c.log.Printf("Contract %v slated for renew is marked not good for renew %v/%v",
					id, ok, oldUtility.GoodForRenew)
				c.contracts.Return(oldContract)
				return
			}
			// Perform the actual renew. If the renew fails, return the
			// contract.
			newContract, err := c.managedRenew(oldContract, amount, endHeight)
			if err != nil {
				c.log.Printf("WARN: failed to renew contract %v: %v\n", id, err)
				c.contracts.Return(oldContract)
				return
			}
			c.log.Printf("Renewed contract %v\n", id)

			// Update the utility values for the new contract, and for the old
			// contract.
			c.mu.Lock()
			newUtility := modules.ContractUtility{
				GoodForUpload: true,
				GoodForRenew:  true,
			}
			if err := c.updateContractUtility(newContract.ID, newUtility); err != nil {
				c.log.Println("Failed to update the contract utilities", err)
				return
			}
			oldUtility.GoodForRenew = false
			oldUtility.GoodForUpload = false
			if err := oldContract.UpdateUtility(oldUtility); err != nil {
				c.log.Println("Failed to update the contract utilities", err)
				return
			}
			c.mu.Unlock()
			// If the contract is a mid-cycle renew, add the contract line to
			// the new contract. The contract line is not included/extended if
			// we are just renewing because the contract is expiring.
			if _, exists := refreshSet[id]; exists {
				// TODO: update PreviousContracts
			}

			// Lock the contractor as we update it to use the new contract
			// instead of the old contract.
			c.mu.Lock()
			defer c.mu.Unlock()
			// Delete the old contract.
			c.contracts.Delete(oldContract)
			// Store the contract in the record of historic contracts.
			c.oldContracts[id] = oldContract.Metadata()
			// Add a mapping from the old contract to the new contract.
			c.renewedIDs[id] = newContract.ID
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
	c.mu.RLock()
	uploadContracts := 0
	for _, id := range c.contracts.IDs() {
		if cu, ok := c.readlockContractUtility(id); ok && cu.GoodForUpload {
			uploadContracts++
		}
	}
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
	for _, contract := range c.contracts.ViewAll() {
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
		c.mu.Lock()
		err = c.updateContractUtility(newContract.ID, modules.ContractUtility{
			GoodForUpload: true,
			GoodForRenew:  true,
		})
		if err != nil {
			c.log.Println("Failed to update the contract utilities", err)
			return
		}
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

// updateContractUtility is a helper function that acquires a contract, updates
// its ContractUtility and returns the contract again.
func (c *Contractor) updateContractUtility(id types.FileContractID, utility modules.ContractUtility) error {
	safeContract, ok := c.contracts.Acquire(id)
	if !ok {
		return errors.New("failed to acquire contract for update")
	}
	defer c.contracts.Return(safeContract)
	return safeContract.UpdateUtility(utility)
}
