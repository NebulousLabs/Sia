package contractor

import (
	"errors"
	"fmt"
	"strings"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/proto"
	"github.com/NebulousLabs/Sia/types"
)

var (
	// the contractor will not form contracts above this price
	maxStoragePrice = types.SiacoinPrecision.Mul64(500e3).Mul(modules.BlockBytesPerMonthTerabyte) // 500k SC / TB / Month
	// the contractor will not download data above this price (3x the maximum monthly storage price)
	maxDownloadPrice = maxStoragePrice.Mul64(3 * 4320)

	errInsufficientAllowance = errors.New("allowance is not large enough to perform contract creation")
	errTooExpensive          = errors.New("host price was too high")
)

// maxSectors is the estimated maximum number of sectors that the allowance
// can support.
func maxSectors(a modules.Allowance, hdb hostDB) (uint64, error) {
	if a.Hosts == 0 || a.Period == 0 {
		return 0, errors.New("invalid allowance")
	}

	// Sample at least 10 hosts.
	nRandomHosts := int(a.Hosts)
	if nRandomHosts < 10 {
		nRandomHosts = 10
	}
	hosts := hdb.RandomHosts(nRandomHosts, nil)
	if len(hosts) < int(a.Hosts) {
		return 0, errors.New("not enough hosts")
	}

	// Calculate cost of storing 1 sector per host for the allowance period.
	var sum types.Currency
	for _, h := range hosts {
		sum = sum.Add(h.StoragePrice)
	}
	averagePrice := sum.Div64(uint64(len(hosts)))
	costPerSector := averagePrice.Mul64(a.Hosts).Mul64(modules.SectorSize).Mul64(uint64(a.Period))

	// Divide total funds by cost per sector.
	numSectors, err := a.Funds.Div(costPerSector).Uint64()
	if err != nil {
		// if there was an overflow, something is definitely wrong
		return 0, errors.New("allowance can fund suspiciously large number of sectors")
	}
	return numSectors, nil
}

// managedNewContract negotiates an initial file contract with the specified
// host, saves it, and returns it.
func (c *Contractor) managedNewContract(host modules.HostDBEntry, numSectors uint64, endHeight types.BlockHeight) (modules.RenterContract, error) {
	// reject hosts that are too expensive
	if host.StoragePrice.Cmp(maxStoragePrice) > 0 {
		return modules.RenterContract{}, errTooExpensive
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

	contract, err := proto.FormContract(params, txnBuilder, c.tpool)
	if err != nil {
		txnBuilder.Drop()
		return modules.RenterContract{}, err
	}
	contractValue := contract.RenterFunds()

	c.mu.Lock()
	c.contracts[contract.ID] = contract
	c.financialMetrics.ContractSpending = c.financialMetrics.ContractSpending.Add(contractValue)
	c.saveSync()
	c.mu.Unlock()

	c.log.Printf("Formed contract with %v for %v SC", host.NetAddress, contractValue.Div(types.SiacoinPrecision))

	return contract, nil
}

// managedFormContracts forms contracts with n hosts using the allowance
// parameters.
func (c *Contractor) managedFormContracts(n int, numSectors uint64, endHeight types.BlockHeight) error {
	if n == 0 {
		return nil
	}

	// Sample at least 10 hosts.
	nRandomHosts := 2 * n
	if nRandomHosts < 10 {
		nRandomHosts = 10
	}
	// Don't select from hosts we've already formed contracts with
	c.mu.RLock()
	var exclude []modules.NetAddress
	for _, contract := range c.contracts {
		exclude = append(exclude, contract.NetAddress)
	}
	c.mu.RUnlock()
	hosts := c.hdb.RandomHosts(nRandomHosts, exclude)
	if len(hosts) < n {
		return errors.New("not enough hosts")
	}

	var numContracts int
	var errs []string
	for _, h := range hosts {
		_, err := c.managedNewContract(h, numSectors, endHeight)
		if err != nil {
			errs = append(errs, fmt.Sprintf("\t%v: %v", h.NetAddress, err))
			continue
		}
		if numContracts++; numContracts >= n {
			break
		}
	}
	// If we couldn't form any contracts, return an error. Otherwise, just log
	// the failures.
	// TODO: is there a better way to handle failure here? Should we prefer an
	// all-or-nothing approach? We can't pick new hosts to negotiate with
	// because they'll probably be more expensive than we can afford.
	if numContracts == 0 {
		return errors.New("could not form any contracts:\n" + strings.Join(errs, "\n"))
	} else if numContracts < n {
		c.log.Printf("WARN: failed to form desired number of contracts (wanted %v, got %v):\n%v", n, numContracts, strings.Join(errs, "\n"))
	}

	return nil
}
