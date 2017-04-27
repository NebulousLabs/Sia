package contractor

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/proto"
	"github.com/NebulousLabs/Sia/types"
)

// managedNewContract negotiates an initial file contract with the specified
// host, saves it, and returns it.
func (c *Contractor) managedNewContract(host modules.HostDBEntry, contractFunds types.Currency, hostCollateral types.Currency, endHeight types.BlockHeight) error {
	// get an address to use for negotiation
	uc, err := c.wallet.NextAddress()
	if err != nil {
		return err
	}

	// create contract params
	c.mu.RLock()
	params := proto.ContractParams{
		Host:          host,
		HostCollateral: hostCollateral,
		RenterFunds:      renterFunds,
		StartHeight:   c.blockHeight,
		EndHeight:     endHeight,
		RefundAddress: uc.UnlockHash(),
	}
	c.mu.RUnlock()

	// Create a transaction builder and begin contract formation.
	txnBuilder := c.wallet.StartTransaction()
	contract, err := proto.FormContract(params, txnBuilder, c.tpool, c.tg.StopChan())
	if err != nil {
		txnBuilder.Drop()
		return err
	}

	c.log.Printf("Formed contract with %v for %v SC", host.PublicKey.String(), contract.TotalCost.Div(types.SiacoinPrecision))
	c.mu.Lock()
	defer c.mu.Unlock()

	// Insert the new contract.
	c.contracts[contract.ID] = contract
	// Update the allowance to account for the change in spending patterns.
	c.allowance.Funds = c.allowance.Funds.Add(contract.TotalCost)
	// Save the changes.
	err = c.saveSync()
	if err != nil {
		c.log.Println("Unable to save the contractor after forming a new file contract")
	}
	return nil // Don't return the save error.
}
