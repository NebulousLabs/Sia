package contractor

// contracts.go handles forming and renewing contracts for the contractor. This
// includes deciding when new contracts need to be formed, when contracts need
// to be renewed, and if contracts need to be blacklisted.

import (
	"errors"
	"fmt"
	"strings"
	"time"

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
func (c *Contractor) contractEndHeight() types.BlockHeight {
	var endHeight types.BlockHeight
	for _, contract := range c.contracts {
		endHeight = contract.EndHeight()
		break
	}
	return endHeight
}

// managedNewContract negotiates an initial file contract with the specified
// host, saves it, and returns it.
func (c *Contractor) managedNewContract(host modules.HostDBEntry, numSectors uint64, endHeight types.BlockHeight) (modules.RenterContract, error) {
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
		Filesize:      numSectors * modules.SectorSize,
		StartHeight:   c.blockHeight,
		EndHeight:     endHeight,
		RefundAddress: uc.UnlockHash(),
	}
	c.mu.RUnlock()

	// create transaction builder
	txnBuilder := c.wallet.StartTransaction()

	contract, err := proto.FormContract(params, txnBuilder, c.tpool, c.tg.StopChan())
	if err != nil {
		txnBuilder.Drop()
		return modules.RenterContract{}, err
	}

	contractValue := contract.RenterFunds()
	c.log.Printf("Formed contract with %v for %v SC", host.NetAddress, contractValue.Div(types.SiacoinPrecision))
	return contract, nil
}

// managedFormAllowanceContracts handles the special case where no contracts
// need to be renewed when setting the allowance.
func (c *Contractor) managedFormAllowanceContracts(n int, numSectors uint64, a modules.Allowance) error {
	if n <= 0 {
		return nil
	}

	// if we're forming contracts but not renewing, the new contracts should
	// have the same endHeight as the existing ones. Otherwise, the endHeight
	// should be a full period.
	c.mu.RLock()
	endHeight := c.blockHeight + a.Period
	if len(c.contracts) > 0 {
		endHeight = c.contractEndHeight()
	}
	c.mu.RUnlock()

	// form the contracts
	formed, err := c.managedFormContracts(n, numSectors, endHeight)
	if err != nil {
		return err
	}

	// Set the allowance and update the contract set
	c.mu.Lock()
	c.allowance = a
	for _, contract := range formed {
		c.contracts[contract.ID] = contract
	}
	err = c.saveSync()
	c.mu.Unlock()

	return err
}

// managedFormContracts forms contracts with n hosts using the allowance
// parameters.
func (c *Contractor) managedFormContracts(n int, numSectors uint64, endHeight types.BlockHeight) ([]modules.RenterContract, error) {
	if n <= 0 {
		return nil, nil
	}

	// Sample at least 10 hosts.
	nRandomHosts := 2 * n
	if nRandomHosts < 10 {
		nRandomHosts = 10
	}
	// Don't select from hosts we've already formed contracts with
	c.mu.RLock()
	var exclude []types.SiaPublicKey
	for _, contract := range c.contracts {
		exclude = append(exclude, contract.HostPublicKey)
	}
	c.mu.RUnlock()
	hosts := c.hdb.RandomHosts(nRandomHosts, exclude)
	if len(hosts) < n {
		return nil, fmt.Errorf("not enough hosts in hostdb for contract formation, got %v but needed %v", len(hosts), n)
	}

	var contracts []modules.RenterContract
	var errs []string
	for _, h := range hosts {
		contract, err := c.managedNewContract(h, numSectors, endHeight)
		if err != nil {
			errs = append(errs, fmt.Sprintf("\t%v: %v", h.NetAddress, err))
			continue
		}
		contracts = append(contracts, contract)
		if len(contracts) >= n {
			break
		}
		// sleep between forming each contract to alleviate potential block
		// propagation issues
		time.Sleep(contractFormationInterval)
	}
	// If we couldn't form any contracts, return an error. Otherwise, just log
	// the failures.
	//
	// TODO: is there a better way to handle failure here? Should we prefer an
	// all-or-nothing approach? We can't pick new hosts to negotiate with
	// because they'll probably be more expensive than we can afford.
	if len(contracts) == 0 {
		return nil, errors.New("could not form any contracts:\n" + strings.Join(errs, "\n"))
	} else if len(contracts) < n {
		c.log.Printf("WARN: failed to form desired number of contracts (wanted %v, got %v):\n%v", n, len(contracts), strings.Join(errs, "\n"))
	}

	return contracts, nil
}

// managedRenew negotiates a new contract for data already stored with a host.
// It returns the new contract. This is a blocking call that performs network
// I/O.
func (c *Contractor) managedRenew(contract modules.RenterContract, numSectors uint64, newEndHeight types.BlockHeight) (modules.RenterContract, error) {
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
		Filesize:      numSectors * modules.SectorSize,
		StartHeight:   c.blockHeight,
		EndHeight:     newEndHeight,
		RefundAddress: uc.UnlockHash(),
	}
	c.mu.RUnlock()

	// execute negotiation protocol
	txnBuilder := c.wallet.StartTransaction()
	newContract, err := proto.Renew(contract, params, txnBuilder, c.tpool, c.tg.StopChan())
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
		newContract, err = proto.Renew(contract, params, txnBuilder, c.tpool, c.tg.StopChan())
	}
	if err != nil {
		txnBuilder.Drop() // return unused outputs to wallet
		return modules.RenterContract{}, err
	}

	return newContract, nil
}

// managedRenewAllowanceContracts contains the logic for renewing contracts
// while setting a new allowance.
func (c *Contractor) managedRenewAllowanceContracts(a modules.Allowance, periodStart, endHeight types.BlockHeight, numSectors uint64, remaining int) (map[types.FileContractID]modules.RenterContract, map[types.FileContractID]types.FileContractID, int) {
	c.mu.RLock()
	// gather contracts to renew
	var renewSet []modules.RenterContract
	for _, contract := range c.contracts {
		renewSet = append(renewSet, contract)
	}

	// calculate new endHeight; if the period has not changed, the endHeight
	// should not change either
	if a.Period == c.allowance.Period && len(c.contracts) > 0 {
		// COMPAT v0.6.0 - old hosts require end height increase by at least 1
		endHeight = c.contractEndHeight() + 1
	}
	c.mu.RUnlock()

	// renew existing contracts with new allowance parameters
	newContracts := make(map[types.FileContractID]modules.RenterContract)
	renewedIDs := make(map[types.FileContractID]types.FileContractID)
	for _, contract := range renewSet {
		newContract, err := c.managedRenew(contract, numSectors, endHeight)
		if err != nil {
			c.log.Printf("WARN: failed to renew contract with %v (error: %v); a new contract will be formed in its place", contract.NetAddress, err)
			remaining++
			continue
		}
		newContracts[newContract.ID] = newContract
		renewedIDs[contract.ID] = newContract.ID
		if len(newContracts) >= int(a.Hosts) {
			break
		}
		// sleep between renewing each contract to alleviate potential block
		// propagation issues
		time.Sleep(contractFormationInterval)
	}
	return newContracts, renewedIDs, remaining
}

// managedRenewContracts renews any contracts that are up for renewal, using
// the current allowance.
func (c *Contractor) managedRenewContracts() error {
	c.mu.RLock()
	// Renew contracts when they enter the renew window.
	// NOTE: offline contracts are not considered here, since we may have
	// replaced them (and we probably won't be able to connect to their host
	// anyway)
	var renewSet []types.FileContractID
	for _, contract := range c.onlineContracts() {
		if c.blockHeight+c.allowance.RenewWindow >= contract.EndHeight() {
			renewSet = append(renewSet, contract.ID)
		}
	}
	c.mu.RUnlock()
	if len(renewSet) == 0 {
		// nothing to do
		return nil
	}

	c.log.Printf("renewing %v contracts", len(renewSet))

	c.mu.RLock()
	endHeight := c.blockHeight + c.allowance.Period
	max, err := maxSectors(c.allowance, c.hdb, c.tpool)
	c.mu.RUnlock()
	if err != nil {
		return err
	}
	// Only allocate half as many sectors as the max. This leaves some leeway
	// for replacing contracts, transaction fees, etc.
	numSectors := max / 2
	// check that this is sufficient to store at least one sector
	if numSectors == 0 {
		return ErrInsufficientAllowance
	}

	// invalidate all active editors/downloaders for the contracts we want to
	// renew
	c.mu.Lock()
	for _, id := range renewSet {
		c.renewing[id] = true
	}
	c.mu.Unlock()

	// after we finish renewing, unset the 'renewing' flag on each contract
	defer func() {
		c.mu.Lock()
		for _, id := range renewSet {
			delete(c.renewing, id)
		}
		c.mu.Unlock()
	}()

	// wait for all active editors and downloaders to finish, then grab the
	// latest revision of each contract
	var oldContracts []modules.RenterContract
	for _, id := range renewSet {
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
		contract, ok := c.contracts[id]
		c.mu.RUnlock()
		if !ok {
			c.log.Printf("WARN: no record of contract previously added to the renew set (ID: %v)", id)
			continue
		}
		oldContracts = append(oldContracts, contract)
	}

	// map old ID to new contract, for easy replacement later
	newContracts := make(map[types.FileContractID]modules.RenterContract)
	for _, contract := range oldContracts {
		newContract, err := c.managedRenew(contract, numSectors, endHeight)
		if err != nil {
			c.log.Printf("WARN: failed to renew contract with %v: %v", contract.NetAddress, err)
		} else {
			newContracts[contract.ID] = newContract
			c.log.Printf("renewed contract %v -> %v", contract.ID, newContract.ID)
		}
		// sleep between renewing each contract to alleviate potential block
		// propagation issues
		time.Sleep(contractFormationInterval)
	}

	// replace old contracts with renewed ones
	c.mu.Lock()
	for oldID, contract := range newContracts {
		// archive the old contract
		if oldContract, ok := c.contracts[oldID]; ok {
			c.oldContracts[oldID] = oldContract
			delete(c.contracts, oldID)
		}
		// insert the new contract
		c.contracts[contract.ID] = contract
		// add a mapping from old->new contract
		c.renewedIDs[oldID] = contract.ID
		// move the cachedRevision entry to the new ID
		c.cachedRevisions[contract.ID] = c.cachedRevisions[oldID]
		delete(c.cachedRevisions, oldID)
	}
	err = c.saveSync()
	c.mu.Unlock()
	return err
}
