package contractor

import (
	"errors"
	"fmt"
	"strings"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/contractor/proto"
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

// newContract negotiates an initial file contract with the specified host,
// saves it, and returns it.
func (c *Contractor) newContract(host modules.HostDBEntry, filesize uint64, duration types.BlockHeight) (proto.Contract, error) {
	// reject hosts that are too expensive
	if host.StoragePrice.Cmp(maxStoragePrice) > 0 {
		return proto.Contract{}, errTooExpensive
	}

	// TODO: move this outside this function?
	c.mu.Lock()
	// get an address to use for negotiation
	if c.cachedAddress == (types.UnlockHash{}) {
		uc, err := c.wallet.NextAddress()
		if err != nil {
			c.mu.Unlock()
			return proto.Contract{}, err
		}
		c.cachedAddress = uc.UnlockHash()
	}
	// create contract params
	params := proto.ContractParams{
		Host:          host,
		Filesize:      filesize,
		StartHeight:   c.blockHeight,
		EndHeight:     c.blockHeight + duration,
		RefundAddress: c.cachedAddress,
	}
	c.mu.Unlock()

	// create transaction builder
	txnBuilder := c.wallet.StartTransaction()

	contract, err := proto.FormContract(params, txnBuilder, c.tpool)
	if err != nil {
		txnBuilder.Drop()
		return proto.Contract{}, err
	}

	c.mu.Lock()
	c.contracts[contract.ID] = contract
	c.cachedAddress = types.UnlockHash{} // clear the cached address
	c.saveSync()
	c.mu.Unlock()

	contractValue := contract.LastRevision.NewValidProofOutputs[0].Value
	c.log.Printf("Formed contract with %v for %v SC", host.NetAddress, contractValue.Div(types.SiacoinPrecision))

	return contract, nil
}

// formContracts forms contracts with hosts using the allowance parameters.
func (c *Contractor) formContracts(a modules.Allowance) error {
	// Sample at least 10 hosts.
	nRandomHosts := 2 * int(a.Hosts)
	if nRandomHosts < 10 {
		nRandomHosts = 10
	}
	hosts := c.hdb.RandomHosts(nRandomHosts, nil)
	if uint64(len(hosts)) < a.Hosts {
		return errors.New("not enough hosts")
	}
	// Calculate average host price.
	var sum types.Currency
	for _, h := range hosts {
		sum = sum.Add(h.StoragePrice)
	}
	avgPrice := sum.Div64(uint64(len(hosts)))

	// Check that allowance is sufficient to store at least one sector per
	// host for the specified duration.
	costPerSector := avgPrice.Mul64(a.Hosts).Mul64(modules.SectorSize).Mul64(uint64(a.Period))
	if a.Funds.Cmp(costPerSector) < 0 {
		return errInsufficientAllowance
	}

	// Calculate the filesize of the contracts by using the average host price
	// and rounding down to the nearest sector.
	numSectors, err := a.Funds.Div(costPerSector).Uint64()
	if err != nil {
		// if there was an overflow, something is definitely wrong
		return errors.New("allowance resulted in unexpectedly large contract size")
	}
	filesize := numSectors * modules.SectorSize

	// Form contracts with each host.
	var numContracts uint64
	var errs []string
	for _, h := range hosts {
		_, err := c.newContract(h, filesize, a.Period)
		if err != nil {
			errs = append(errs, fmt.Sprintf("\t%v: %v", h.NetAddress, err))
			continue
		}
		if numContracts++; numContracts >= a.Hosts {
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
	} else if numContracts < a.Hosts {
		c.log.Printf("WARN: failed to form desired number of contracts (wanted %v, got %v): %v", a.Hosts, numContracts, strings.Join(errs, "\n"))
	}
	c.mu.Lock()
	c.renewHeight = c.blockHeight + a.Period // TODO: this risks desync
	c.mu.Unlock()
	return nil
}
