package contractor

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/proto"
	"github.com/NebulousLabs/Sia/types"
)

var (
	// the contractor will not form contracts above this price
	maxStoragePrice = types.SiacoinPrecision.Mul64(500e3).Div(modules.BlockBytesPerMonthTerabyte) // 500k SC / TB / Month
	// the contractor will not download data above this price (3x the maximum monthly storage price)
	maxDownloadPrice = maxStoragePrice.Mul64(3 * 4320)
	// the contractor will cap host's MaxCollateral setting to this value
	maxCollateral = types.SiacoinPrecision.Mul64(1e3) // 1k SC

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

func initialContractMetrics(contract modules.RenterContract, host modules.HostDBEntry, txn types.Transaction) modules.RenterContractMetrics {
	metrics := modules.RenterContractMetrics{
		ID:          contract.ID,
		ContractFee: host.ContractPrice,
		SiafundFee:  types.Tax(contract.EndHeight(), contract.FileContract.Payout),
		Unspent:     contract.RenterFunds(),
	}
	for _, fee := range txn.MinerFees {
		metrics.TxnFee = metrics.TxnFee.Add(fee)
	}
	metrics.TotalCost = metrics.Unspent.Add(metrics.ContractFee).Add(metrics.TxnFee).Add(metrics.SiafundFee)
	return metrics
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
	currentHeight := c.blockHeight
	params := proto.ContractParams{
		Host:          host,
		Filesize:      numSectors * modules.SectorSize,
		StartHeight:   currentHeight,
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
	// add metrics entry for contract
	txn, _ := txnBuilder.View()
	metrics := initialContractMetrics(contract, host, txn)
	c.mu.Lock()
	c.contractMetrics[contract.ID] = metrics
	c.mu.Unlock()

	contractValue := contract.RenterFunds()
	c.log.Printf("Formed contract with %v for %v SC", host.NetAddress, contractValue.Div(types.SiacoinPrecision))

	return contract, nil
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
	var exclude []modules.NetAddress
	for _, contract := range c.contracts {
		exclude = append(exclude, contract.NetAddress)
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
		if build.Release != "testing" {
			// sleep for 1 minute to alleviate potential block propagation issues
			time.Sleep(60 * time.Second)
		}
	}
	// If we couldn't form any contracts, return an error. Otherwise, just log
	// the failures.
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
