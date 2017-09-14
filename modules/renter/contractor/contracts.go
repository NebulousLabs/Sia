package contractor

// contracts.go handles forming and renewing contracts for the contractor. This
// includes deciding when new contracts need to be formed, when contracts need
// to be renewed, and if contracts need to be blacklisted.

import (
	"errors"
	"fmt"
	"math/big"
	"time"

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

// maxSectors is the estimated maximum number of sectors that the allowance
// can support.
func maxSectors(a modules.Allowance, hdb hostDB, tp transactionPool) (uint64, error) {
	if a.Hosts <= 0 || a.Period <= 0 {
		return 0, errors.New("invalid allowance")
	}

	// Sample at least 10 hosts.
	nRandomHosts := int(a.Hosts)
	if nRandomHosts < minHostsForEstimations {
		nRandomHosts = minHostsForEstimations
	}
	hosts := hdb.RandomHosts(nRandomHosts, nil)
	if len(hosts) < int(a.Hosts) {
		return 0, fmt.Errorf("not enough hosts in hostdb for sector calculation, got %v but needed %v", len(hosts), int(a.Hosts))
	}

	// Calculate cost of creating contracts with each host, and the cost of
	// storing sectors on each host.
	var sectorSum types.Currency
	var contractCostSum types.Currency
	for _, h := range hosts {
		sectorSum = sectorSum.Add(h.StoragePrice)
		contractCostSum = contractCostSum.Add(h.ContractPrice)
	}
	averageSectorPrice := sectorSum.Div64(uint64(len(hosts)))
	averageContractPrice := contractCostSum.Div64(uint64(len(hosts)))
	costPerSector := averageSectorPrice.Mul64(a.Hosts).Mul64(modules.SectorSize).Mul64(uint64(a.Period))
	costForContracts := averageContractPrice.Mul64(a.Hosts)

	// Subtract fees for creating the file contracts from the allowance.
	_, feeEstimation := tp.FeeEstimation()
	costForTxnFees := types.NewCurrency64(estimatedFileContractTransactionSize).Mul(feeEstimation).Mul64(a.Hosts)
	// Check for potential divide by zero
	if a.Funds.Cmp(costForTxnFees.Add(costForContracts)) <= 0 {
		return 0, ErrInsufficientAllowance
	}
	sectorFunds := a.Funds.Sub(costForTxnFees).Sub(costForContracts)

	// Divide total funds by cost per sector.
	numSectors, err := sectorFunds.Div(costPerSector).Uint64()
	if err != nil {
		return 0, errors.New("error when totaling number of sectors that can be bought with an allowance: " + err.Error())
	}
	return numSectors, nil
}

// contractEndHeight returns the height at which the Contractor's contracts
// end. If there are no contracts, it returns zero.
//
// TODO: The contract end height should be picked based on the current period
// start plus the period duration, not based on the end heights of the existing
// contracts.
func (c *Contractor) contractEndHeight() types.BlockHeight {
	var endHeight types.BlockHeight
	for _, contract := range c.contracts {
		endHeight = contract.EndHeight()
		break
	}
	return endHeight
}

// managedMarkContractsUtility checks every active contract in the contractor and
// figures out whether the contract is useful for uploading, and whehter the
// contract should be renewed.
func (c *Contractor) managedMarkContractsUtility() {
	// Pull a new set of hosts from the hostdb that could be used as a new set
	// to match the allowance. The lowest scoring host of these new hosts will
	// be used as a baseline for determining whether our existing contracts are
	// worthwhile.
	c.mu.RLock()
	hostCount := int(c.allowance.Hosts)
	c.mu.RUnlock()
	if hostCount <= 0 {
		return
	}
	hosts := c.hdb.RandomHosts(hostCount+minScoreHostBuffer, nil)
	if len(hosts) <= 0 {
		return
	}

	// Find the lowest score of this batch of hosts.
	lowestScore := c.hdb.ScoreBreakdown(hosts[0]).Score
	for i := 1; i < len(hosts); i++ {
		score := c.hdb.ScoreBreakdown(hosts[i]).Score
		if score.Cmp(lowestScore) < 0 {
			lowestScore = score
		}
	}
	// Set the minimum acceptable score to a factor of the lowest score.
	minScore := lowestScore.Div(scoreLeeway)

	// Pull together the set of contracts.
	c.mu.RLock()
	contracts := make([]modules.RenterContract, 0, len(c.contracts))
	for _, contract := range c.contracts {
		contracts = append(contracts, contract)
	}
	c.mu.RUnlock()

	// Go through and figure out if the utility fields need to be changed.
	for i := 0; i < len(contracts); i++ {
		// Start the contract in good standing.
		contracts[i].GoodForUpload = true
		contracts[i].GoodForRenew = true

		host, exists := c.hdb.Host(contracts[i].HostPublicKey)
		// Contract has no utility if the host is not in the database.
		if !exists {
			contracts[i].GoodForUpload = false
			contracts[i].GoodForRenew = false
			continue
		}
		// Contract has no utility if the score is poor.
		if c.hdb.ScoreBreakdown(host).Score.Cmp(minScore) < 0 {
			contracts[i].GoodForUpload = false
			contracts[i].GoodForRenew = false
			continue
		}
		// Contract has no utility if the host is offline.
		c.mu.Lock()
		offline := c.isOffline(contracts[i].ID)
		c.mu.Unlock()
		if offline {
			contracts[i].GoodForUpload = false
			contracts[i].GoodForRenew = false
			continue
		}
		// Contract has no utility if renew has already completed. (grab some
		// extra values while we have the mutex)
		c.mu.RLock()
		blockHeight := c.blockHeight
		renewWindow := c.allowance.RenewWindow
		_, renewedPreviously := c.renewedIDs[contracts[i].ID]
		c.mu.RUnlock()
		if renewedPreviously {
			contracts[i].GoodForUpload = false
			contracts[i].GoodForRenew = false
			continue
		}

		// Contract should not be used for upload if the number of Merkle roots
		// exceeds 25e3 - this is in place because the current hosts do not
		// really perform well beyond this number of sectors in a single
		// contract. Future updates will fix this, at which point this limit
		// will change and also have to switch based on host version.
		if len(contracts[i].MerkleRoots) > 25e3 {
			// Contract is still fine to be renewed, we just shouldn't keep
			// adding data to this contract.
			contracts[i].GoodForUpload = false
			continue
		}
		// Contract should not be used for uploading if the time has come to
		// renew the contract.
		if blockHeight+renewWindow >= contracts[i].EndHeight() {
			contracts[i].GoodForUpload = false
			continue
		}
	}

	// Update the contractor to reflect the new state for each of the contracts.
	c.mu.Lock()
	for i := 0; i < len(contracts); i++ {
		contract, exists := c.contracts[contracts[i].ID]
		if !exists {
			continue
		}
		contract.GoodForUpload = contracts[i].GoodForUpload
		contract.GoodForRenew = contracts[i].GoodForRenew
		c.contracts[contracts[i].ID] = contract
	}
	c.mu.Unlock()
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

	contract, err := proto.FormContract(params, txnBuilder, c.tpool, c.hdb, c.tg.StopChan())
	if err != nil {
		txnBuilder.Drop()
		return modules.RenterContract{}, err
	}

	contractValue := contract.RenterFunds()
	c.log.Printf("Formed contract with %v for %v", host.NetAddress, contractValue.HumanString())
	return contract, nil
}

// managedRenew negotiates a new contract for data already stored with a host.
// It returns the new contract. This is a blocking call that performs network
// I/O.
func (c *Contractor) managedRenew(contract modules.RenterContract, contractFunding types.Currency, newEndHeight types.BlockHeight) (modules.RenterContract, error) {
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
	// Set the net address of the contract to the most recent net address for
	// the host.
	contract.NetAddress = host.NetAddress

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
	newContract, err := proto.Renew(contract, params, txnBuilder, c.tpool, c.hdb, c.tg.StopChan())
	if proto.IsRevisionMismatch(err) {
		// return unused outputs to wallet
		txnBuilder.Drop()
		// try again with the cached revision
		c.mu.RLock()
		cached, ok := c.cachedRevisions[contract.ID]
		c.mu.RUnlock()
		if !ok {
			// nothing we can do; return original error
			c.log.Printf("wanted to recover contract %v with host %v, but no revision was cached", contract.ID, contract.NetAddress)
			return modules.RenterContract{}, err
		}
		c.log.Printf("host %v has different revision for %v; retrying with cached revision", contract.NetAddress, contract.ID)
		contract.LastRevision = cached.Revision
		// need to start a new transaction
		txnBuilder = c.wallet.StartTransaction()
		newContract, err = proto.Renew(contract, params, txnBuilder, c.tpool, c.hdb, c.tg.StopChan())
	}
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
func (c *Contractor) threadedContractMaintenance() {
	// Threading protection.
	err := c.tg.Add()
	if err != nil {
		return
	}
	defer c.tg.Done()
	// Nohting to do if there are no hosts.
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
	} else {
		if !c.maintenanceLock.TryLock() {
			return
		}
	}
	defer c.maintenanceLock.Unlock()

	// Update the utility fields for this contract based on the most recent
	// hostdb.
	c.managedMarkContractsUtility()

	// Figure out which contracts need to be renewed, and while we have the
	// lock, figure out the end height for the new contracts and also the amount
	// to spend on each contract.
	//
	// refreshSet is used to mark contracts that need to be refreshed because
	// they have run out of money. The refreshSet indicates how much currency
	// was used in the previous contract which got exhausted, so that we know to
	// put more money towards the refreshed contract.
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
		//
		// TODO: End height should be calculated using the billing cycle, and
		// not by just adding the period to the current height.
		endHeight = c.blockHeight + c.allowance.Period

		// Determine how many funds have been used already in this billing
		// cycle, and how many funds are remaining. We have to calculate these
		// numbers separately to avoid underflow, and then re-join them later to
		// get the full picture for how many funds are available.
		var fundsUsed types.Currency
		for _, contract := range c.contracts {
			// Calculate the cost of the contract line.
			contractLineCost := contract.TotalCost
			for _, pre := range contract.PreviousContracts {
				contractLineCost = contractLineCost.Add(pre.TotalCost)
			}

			// Check if the contract is expiring. The funds in the contract are
			// handled differently based on this information.
			if c.blockHeight+c.allowance.RenewWindow >= contract.EndHeight() {
				// The contract is expiring. Some of the funds are locked down
				// to renew the contract, and then the remaining funds can be
				// allocated to 'availableFunds'.
				fundsUsed = fundsUsed.Add(contractLineCost).Sub(contract.RenterFunds())
				fundsAvailable = fundsAvailable.Add(contract.RenterFunds())
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
		for _, contract := range c.contracts {
			if !contract.GoodForRenew {
				continue
			}
			if c.blockHeight+c.allowance.RenewWindow >= contract.EndHeight() {
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
				// money that was allocated (the RenterFunds()).
				renewAmount := contract.TotalCost.Sub(contract.ContractFee).Sub(contract.TxnFee).Sub(contract.SiafundFee).Sub(contract.RenterFunds())
				for _, pre := range contract.PreviousContracts {
					renewAmount = renewAmount.Add(pre.TotalCost).Sub(pre.ContractFee).Sub(pre.TxnFee).Sub(pre.SiafundFee).Sub(contract.RenterFunds())
				}

				// Get an estimate for how much the fees will cost.
				//
				// TODO: A better estimate would come from looking at the host
				// and hostdb, and calculating the actual cost of renewing with
				// the host. The current method doesn't even take into account
				// potential discounts from not renewing as much money.
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
				// check if the contract has exhausted its funding and requires premature renewal.
				c.mu.RUnlock()
				host, _ := c.hdb.Host(contract.HostPublicKey)
				c.mu.RLock()

				// skip this host if its prices are too high. managedMarkContractsUtility
				// should make this redundant, but this is here for extra safety.
				if host.StoragePrice.Cmp(maxStoragePrice) > 0 || host.UploadBandwidthPrice.Cmp(maxUploadPrice) > 0 {
					continue
				}

				blockBytes := types.NewCurrency64(modules.SectorSize * uint64(contract.EndHeight()-c.blockHeight))
				sectorStoragePrice := host.StoragePrice.Mul(blockBytes)
				sectorBandwidthPrice := host.UploadBandwidthPrice.Mul64(modules.SectorSize)
				sectorPrice := sectorStoragePrice.Add(sectorBandwidthPrice)
				percentRemaining, _ := big.NewRat(0, 1).SetFrac(contract.RenterFunds().Big(), contract.TotalCost.Big()).Float64()
				if contract.RenterFunds().Cmp(sectorPrice.Mul64(3)) < 0 || percentRemaining < minContractFundRenewalThreshold {
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

			c.mu.RLock()
			oldContract, ok := c.contracts[id]
			c.mu.RUnlock()
			if !ok {
				c.log.Println("WARN: no record of contract previously added to the renew set:", id)
				return
			}

			// Create the new contract.
			newContract, err := c.managedRenew(oldContract, amount, endHeight)
			if err != nil {
				c.log.Printf("WARN: failed to renew contract %v with %v: %v\n", id, oldContract.NetAddress, err)
				return
			}
			c.log.Printf("Renewed contract %v with %v\n", id, oldContract.NetAddress)
			// Update the utility values for the new contract, and for the old
			// contract.
			newContract.GoodForUpload = true
			newContract.GoodForRenew = true
			oldContract.GoodForRenew = false
			oldContract.GoodForUpload = false
			// If the contract is a mid-cycle renew, add the contract line to
			// the new contract. The contract line is not included/extended if
			// we are just renewing because the contract is expiring.
			if _, exists := refreshSet[id]; exists {
				newContract.PreviousContracts = oldContract.PreviousContracts
				oldContract.PreviousContracts = nil
				newContract.PreviousContracts = append(newContract.PreviousContracts, oldContract)
			}

			// Lock the contractor as we update it to use the new contract
			// instead of the old contract.
			c.mu.Lock()
			defer c.mu.Unlock()

			// Store the contract in the record of historic contracts.
			_, exists := c.contracts[oldContract.ID]
			if exists {
				c.oldContracts[oldContract.ID] = oldContract
				delete(c.contracts, oldContract.ID)
			}

			// Add the new contract, including a mapping from the old
			// contract to the new contract.
			c.contracts[newContract.ID] = newContract
			c.renewedIDs[oldContract.ID] = newContract.ID
			c.cachedRevisions[newContract.ID] = c.cachedRevisions[oldContract.ID]
			delete(c.cachedRevisions, oldContract.ID)

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
		case <-time.After(contractFormationInterval):
		}
	}

	// Quit in the event of shutdown.
	select {
	case <-c.tg.StopChan():
		return
	default:
	}

	// Count the number of contracts which are good for uploading, and then make
	// more as needed to fill the gap.
	// Renew any contracts that need to be renewed.
	c.mu.RLock()
	uploadContracts := 0
	for _, contract := range c.contracts {
		if contract.GoodForUpload || (contract.GoodForRenew && c.blockHeight+c.allowance.RenewWindow >= contract.EndHeight()) {
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
	for _, contract := range c.contracts {
		exclude = append(exclude, contract.HostPublicKey)
	}
	initialContractFunds := c.allowance.Funds.Div64(c.allowance.Hosts).Div64(3)
	c.mu.RUnlock()
	hosts := c.hdb.RandomHosts(neededContracts*2+10, exclude)

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
		newContract.GoodForUpload = true
		newContract.GoodForRenew = true

		// Add this contract to the contractor and save.
		c.mu.Lock()
		c.contracts[newContract.ID] = newContract
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
		case <-time.After(contractFormationInterval):
		}
	}
}
