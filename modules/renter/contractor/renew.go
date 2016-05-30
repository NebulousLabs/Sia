package contractor

import (
	"errors"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/proto"
	"github.com/NebulousLabs/Sia/types"
)

// managedRenew negotiates a new contract for data already stored with a host.
// It returns the ID of the new contract. This is a blocking call that
// performs network I/O.
func (c *Contractor) managedRenew(contract modules.RenterContract, numSectors uint64, newEndHeight types.BlockHeight) (types.FileContractID, error) {
	host, ok := c.hdb.Host(contract.NetAddress)
	if !ok {
		return types.FileContractID{}, errors.New("no record of that host")
	} else if host.StoragePrice.Cmp(maxStoragePrice) > 0 {
		return types.FileContractID{}, errTooExpensive
	}

	// get an address to use for negotiation
	uc, err := c.wallet.NextAddress()
	if err != nil {
		return types.FileContractID{}, err
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
		return types.FileContractID{}, err
	}

	// replace old contract with renewed contract
	c.mu.Lock()
	c.contracts[newContract.ID] = newContract
	delete(c.contracts, contract.ID)
	err = c.saveSync()
	c.mu.Unlock()
	if err != nil {
		c.log.Println("WARN: failed to save the contractor:", err)
	}

	return newContract.ID, nil
}

// managedRenewContracts renews any contracts that are up for renewal, using
// the current allowance.
func (c *Contractor) managedRenewContracts() {
	c.mu.RLock()
	// Renew contracts when they enter the renew window.
	var renewSet []modules.RenterContract
	for _, contract := range c.contracts {
		if c.blockHeight+c.allowance.RenewWindow >= contract.EndHeight() {
			renewSet = append(renewSet, contract)
		}
	}
	endHeight := c.blockHeight + c.allowance.Period

	numSectors, err := maxSectors(c.allowance, c.hdb)
	c.mu.RUnlock()
	if err != nil {
		c.log.Println("WARN: could not calculate number of sectors allowance can support:", err)
		return
	}

	if len(renewSet) == 0 {
		// nothing to do
		return
	} else if numSectors == 0 {
		c.log.Printf("WARN: want to renew %v contracts, but allowance is too small", len(renewSet))
		return
	}

	for _, contract := range renewSet {
		_, err := c.managedRenew(contract, numSectors, endHeight)
		if err != nil {
			c.log.Println("WARN: failed to renew contract", contract.ID)
		}
	}
}
