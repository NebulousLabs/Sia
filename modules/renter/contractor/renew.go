package contractor

import (
	"errors"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// Renew negotiates a new contract for data already stored with a host. It
// returns the ID of the new contract. This is a blocking call that performs
// network I/O.
func (c *Contractor) Renew(fcid types.FileContractID, newEndHeight types.BlockHeight) (types.FileContractID, error) {
	c.mu.RLock()
	height := c.blockHeight
	hc, ok := c.contracts[fcid]
	host, eok := c.hdb.Host(hc.IP)
	c.mu.RUnlock()
	if !ok {
		return types.FileContractID{}, errors.New("no record of that contract")
	} else if !eok {
		return types.FileContractID{}, errors.New("no record of that host")
	} else if newEndHeight < height {
		return types.FileContractID{}, errors.New("cannot renew below current height")
	} else if host.Price.Cmp(maxPrice) > 0 {
		return types.FileContractID{}, errTooExpensive
	}

	// get an address to use for negotiation
	c.mu.Lock()
	if c.cachedAddress == (types.UnlockHash{}) {
		uc, err := c.wallet.NextAddress()
		if err != nil {
			c.mu.Unlock()
			return types.FileContractID{}, err
		}
		c.cachedAddress = uc.UnlockHash()
	}
	ourAddress := c.cachedAddress
	c.mu.Unlock()

	renterCost := host.Price.Mul(types.NewCurrency64(hc.LastRevision.NewFileSize)).Mul(types.NewCurrency64(uint64(newEndHeight - height)))
	renterCost = renterCost.MulFloat(1.05) // extra buffer to guarantee we won't run out of money during revision
	payout := renterCost                   // no collateral

	// create file contract
	fc := types.FileContract{
		FileSize:       hc.LastRevision.NewFileSize,
		FileMerkleRoot: hc.LastRevision.NewFileMerkleRoot,
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

	// create transaction builder
	txnBuilder := c.wallet.StartTransaction()

	// initiate connection
	conn, err := c.dialer.DialTimeout(hc.IP, 15*time.Second)
	if err != nil {
		return types.FileContractID{}, err
	}
	defer conn.Close()
	if err := encoding.WriteObject(conn, modules.RPCRenew); err != nil {
		return types.FileContractID{}, errors.New("couldn't initiate RPC: " + err.Error())
	}
	if err := encoding.WriteObject(conn, fcid); err != nil {
		return types.FileContractID{}, errors.New("couldn't send contract ID: " + err.Error())
	}

	// execute negotiation protocol
	newContract, err := negotiateContract(conn, hc.IP, fc, txnBuilder, c.tpool)
	if err != nil {
		txnBuilder.Drop() // return unused outputs to wallet
		return types.FileContractID{}, err
	}

	// update host contract
	c.mu.Lock()
	c.contracts[newContract.ID] = newContract
	c.cachedAddress = types.UnlockHash{} // clear cachedAddress
	err = c.save()
	c.mu.Unlock()
	if err != nil {
		c.log.Println("WARN: failed to save the contractor:", err)
	}

	return newContract.ID, nil
}
