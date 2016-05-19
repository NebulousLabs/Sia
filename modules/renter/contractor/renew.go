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
// TODO: take an allowance and renew with those parameters
func (c *Contractor) managedRenew(contract modules.RenterContract, filesize uint64, newEndHeight types.BlockHeight) (types.FileContractID, error) {
	c.mu.RLock()
	height := c.blockHeight
	c.mu.RUnlock()
	if newEndHeight < height {
		return types.FileContractID{}, errors.New("cannot renew below current height")
	}
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
		Filesize:      filesize,
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

	// update host contract
	c.mu.Lock()
	c.contracts[newContract.ID] = newContract
	err = c.saveSync()
	c.mu.Unlock()
	if err != nil {
		c.log.Println("WARN: failed to save the contractor:", err)
	}

	return newContract.ID, nil
}

// threadedRenewContracts renews the Contractor's contracts according to the
// specified allowance and at the specified height.
func (c *Contractor) threadedRenewContracts(allowance modules.Allowance, newHeight types.BlockHeight) {
	// calculate filesize using new allowance
	contracts := c.Contracts()
	var sum types.Currency
	var numHosts uint64
	for _, contract := range contracts {
		if h, ok := c.hdb.Host(contract.NetAddress); ok {
			sum = sum.Add(h.StoragePrice)
			numHosts++
		}
	}
	if numHosts == 0 || numHosts < allowance.Hosts {
		// ??? get more
		return
	}
	avgPrice := sum.Div64(numHosts)

	costPerSector := avgPrice.Mul64(allowance.Hosts).Mul64(modules.SectorSize).Mul64(uint64(allowance.Period))

	if allowance.Funds.Cmp(costPerSector) < 0 {
		// errors.New("insufficient funds")
	}

	// Calculate the filesize of the contracts by using the average host price
	// and rounding down to the nearest sector.
	numSectors, err := allowance.Funds.Div(costPerSector).Uint64()
	if err != nil {
		// errors.New("allowance resulted in unexpectedly large contract size")
	}
	filesize := numSectors * modules.SectorSize

	for _, contract := range contracts {
		if contract.FileContract.WindowStart < newHeight {
			_, err := c.managedRenew(contract, filesize, newHeight)
			if err != nil {
				c.log.Println("WARN: failed to renew contract", contract.ID, ":", err)
			}
		}
	}

	// TODO: reset renewHeight if too many rewewals failed.
	// TODO: form more contracts if numRenewed < allowance.Hosts
}
