package contractor

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/proto"
	"github.com/NebulousLabs/Sia/types"
)

// managedRenew negotiates a new contract for data already stored with a host.
// It returns the new contract. This is a blocking call that performs network
// I/O.
//
// The contractFunds should be the target amount to spend inside of the contract.
// The fees portion of the contract will be managed automatically based on the
// host settings and on the provided host collateral (collateral is provided
// because it can be set to lower than the maximum offered by the host, reducing
// fees).
func (c *Contractor) managedRenewContract(contract modules.RenterContract, host modules.HostDBEntry, contractFunds types.Currency, hostCollateral types.Currency, newEndHeight types.BlockHeight) {
	// Sanity check - the public key of the host should match the public key of
	// the contract.
	if contract.HostPublicKey.String() != host.PublicKey.String() {
		build.Critical("Renew called with non-matching contract and host")
	}
	// Set the net address of the contract to the most recent net address for
	// the host.
	contract.NetAddress = host.NetAddress

	// Get an address to be used in negotiation.
	uc, err := c.wallet.NextAddress()
	if err != nil {
		return
	}

	// create contract params
	c.mu.RLock()
	params := proto.ContractParams{
		Host:           host,
		HostCollateral: hostCollateral,
		ContractFunds:  contractFunds,
		StartHeight:    c.blockHeight,
		EndHeight:      newEndHeight,
		RefundAddress:  uc.UnlockHash(),
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
			return
		}
		c.log.Printf("host %v has different revision for %v; retrying with cached revision", contract.NetAddress, contract.ID)
		contract.LastRevision = cached.Revision
		// need to start a new transaction
		txnBuilder = c.wallet.StartTransaction()
		newContract, err = proto.Renew(contract, params, txnBuilder, c.tpool, c.tg.StopChan())
	}
	if err != nil {
		txnBuilder.Drop() // return unused outputs to wallet
		return
	}

	// Success, update the set of contracts in the contractor.
	c.log.Printf("Renewed a contract with %v for %v SC\n", host.PublicKey.String(), contract.TotalCost.Div(types.SiacoinPrecision))
	c.mu.Lock()
	defer c.mu.Unlock()

	// Establish the new contract as in good standing, copy the upload
	// usefulness from the old contract.
	newContract.InGoodStanding = true
	newContract.UsefulForUpload = contract.UsefulForUpload
	// Archive the old contract.
	c.oldContracts[contract.ID] = contract
	// Delete the old contract.
	delete(c.contracts, contract.ID)
	// Insert the new contract.
	c.contracts[newContract.ID] = newContract
	// Map from the old contract to the new contract.
	c.renewedIDs[contract.ID] = newContract.ID
	// Transfer the current cached revision to the new contract id.
	c.cachedRevisions[newContract.ID] = c.cachedRevisions[contract.ID] // TODO: Is this necessary, won't the revision numbers, etc. be off anyway?
	// Delete the legacy cached revision.
	delete(c.cachedRevisions, contract.ID)
	// Update the allowance to account for the change in spending patterns.
	c.allowance.Funds = c.allowance.Funds.Sub(contract.TotalCost).Add(newContract.TotalCost)
	// Save the changes.
	err = c.saveSync()
	if err != nil {
		c.log.Println("Unable to save the contractor after renewing a contract:", err)
	}
	return
}
