package contractor

import (
	"errors"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// managedRenew negotiates a new contract for data already stored with a host.
// It returns the ID of the new contract. This is a blocking call that
// performs network I/O.
// TODO: take an allowance and renew with those parameters
func (c *Contractor) managedRenew(contract Contract, filesize uint64, newEndHeight types.BlockHeight) (types.FileContractID, error) {
	c.mu.RLock()
	height := c.blockHeight
	c.mu.RUnlock()
	if newEndHeight < height {
		return types.FileContractID{}, errors.New("cannot renew below current height")
	}
	host, ok := c.hdb.Host(contract.IP)
	if !ok {
		return types.FileContractID{}, errors.New("no record of that host")
	} else if host.ContractPrice.Cmp(maxPrice) > 0 {
		return types.FileContractID{}, errTooExpensive
	}

	// get an address to use for negotiation
	var ourAddress types.UnlockHash
	err := func() error {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.cachedAddress == (types.UnlockHash{}) {
			uc, err := c.wallet.NextAddress()
			if err != nil {
				return err
			}
			c.cachedAddress = uc.UnlockHash()
		}
		ourAddress = c.cachedAddress
		return nil
	}()
	if err != nil {
		return types.FileContractID{}, err
	}

	// TODO: what if this isn't enough money??
	renterCost := host.ContractPrice.Mul(types.NewCurrency64(filesize)).Mul(types.NewCurrency64(uint64(newEndHeight - height)))
	payout := renterCost // no collateral

	// create file contract
	fc := types.FileContract{
		FileSize:       contract.LastRevision.NewFileSize, // filesize is not modified; only the payout is
		FileMerkleRoot: contract.LastRevision.NewFileMerkleRoot,
		WindowStart:    newEndHeight,
		WindowEnd:      newEndHeight + host.WindowSize,
		Payout:         payout,
		UnlockHash:     types.UnlockHash{}, // to be filled in by negotiateContract
		RevisionNumber: 0,
		ValidProofOutputs: []types.SiacoinOutput{
			// nothing returned to us; everything goes to the host
			{Value: types.ZeroCurrency, UnlockHash: ourAddress},
			{Value: types.PostTax(height, renterCost), UnlockHash: host.UnlockHash},
		},
		MissedProofOutputs: []types.SiacoinOutput{
			// nothing returned to us; everything goes to the void
			{Value: types.ZeroCurrency, UnlockHash: ourAddress},
			{Value: types.PostTax(height, renterCost), UnlockHash: types.UnlockHash{}},
		},
	}

	// initiate connection
	conn, err := c.dialer.DialTimeout(contract.IP, 15*time.Second)
	if err != nil {
		return types.FileContractID{}, err
	}
	defer conn.Close()
	if err := encoding.WriteObject(conn, modules.RPCRenew); err != nil {
		return types.FileContractID{}, errors.New("couldn't initiate RPC: " + err.Error())
	}

	// verify the host's settings and confirm its identity
	host, err = verifySettings(conn, host, c.hdb)
	if err != nil {
		return types.FileContractID{}, err
	}

	// send contract ID
	if err := encoding.WriteObject(conn, contract.ID); err != nil {
		return types.FileContractID{}, errors.New("couldn't send contract ID: " + err.Error())
	}

	// read missing sector roots
	var missingSectors []crypto.Hash
	if err := encoding.ReadObject(conn, &missingSectors, 2048); err != nil { // ?? length?
		return types.FileContractID{}, errors.New("couldn't read missing sectors: " + err.Error())
	}
	if len(missingSectors) != 0 {
		// TODO: reduce payouts? reupload sectors?
	}

	// create transaction builder
	txnBuilder := c.wallet.StartTransaction()

	// execute negotiation protocol
	newContract, err := negotiateContract(conn, host, fc, txnBuilder, c.tpool)
	if err != nil {
		txnBuilder.Drop() // return unused outputs to wallet
		return types.FileContractID{}, err
	}

	// update host contract
	c.mu.Lock()
	c.contracts[newContract.ID] = newContract
	c.spentPeriod = c.spentPeriod.Add(fc.Payout)
	c.spentTotal = c.spentTotal.Add(fc.Payout)
	c.cachedAddress = types.UnlockHash{} // clear cachedAddress
	err = c.save()
	c.mu.Unlock()
	if err != nil {
		c.log.Println("WARN: failed to save the contractor:", err)
	}

	// Delete a sector from the old contract, for anonymity reasons. (We don't
	// want the same Merkle root to appear in the blockchain for every
	// renewal.) Not a big deal if this fails, as long as it's not failing
	// every time.
	go func() {
		he, err := c.Editor(contract)
		if err == nil {
			err = he.Delete(contract.MerkleRoots[0])
			he.Close()
		}
		if err != nil {
			c.log.Println("WARN: couldn't delete sector from expired contract:", err)
		}
	}()

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
		if h, ok := c.hdb.Host(contract.IP); ok {
			sum = sum.Add(h.ContractPrice)
			numHosts++
		}
	}
	if numHosts < allowance.Hosts {
		// ??? get more
	}
	avgPrice := sum.Div(types.NewCurrency64(numHosts))

	costPerSector := avgPrice.
		Mul(types.NewCurrency64(allowance.Hosts)).
		Mul(types.NewCurrency64(modules.SectorSize)).
		Mul(types.NewCurrency64(uint64(allowance.Period)))
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
