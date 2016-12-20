package contractor

import (
	"errors"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/proto"
	"github.com/NebulousLabs/Sia/types"
)

// managedRenew negotiates a new contract for data already stored with a host.
// It returns the new contract. This is a blocking call that performs network
// I/O.
func (c *Contractor) managedRenew(contract modules.RenterContract, numSectors uint64, newEndHeight types.BlockHeight) (modules.RenterContract, error) {
	host, ok := c.hdb.Host(contract.NetAddress)
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
		Filesize:      numSectors * modules.SectorSize,
		StartHeight:   c.blockHeight,
		EndHeight:     newEndHeight,
		RefundAddress: uc.UnlockHash(),
	}
	c.mu.RUnlock()

	txnBuilder := c.wallet.StartTransaction()

	// execute negotiation protocol
	newContract, err := proto.Renew(contract, params, txnBuilder, c.tpool)
	if err != nil {
		txnBuilder.Drop() // return unused outputs to wallet
		return modules.RenterContract{}, err
	}

	return newContract, nil
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
		}
		if build.Release != "testing" {
			// sleep for 1 minute to alleviate potential block propagation issues
			time.Sleep(60 * time.Second)
		}
	}

	// replace old contracts with renewed ones
	c.mu.Lock()
	for id, contract := range newContracts {
		delete(c.contracts, id)
		c.contracts[contract.ID] = contract
		c.renewedIDs[id] = contract.ID
	}
	err = c.saveSync()
	c.mu.Unlock()
	return err
}
