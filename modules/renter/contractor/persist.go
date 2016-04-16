package contractor

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// contractorPersist defines what Contractor data persists across sessions.
type contractorPersist struct {
	Allowance   modules.Allowance
	Contracts   []Contract
	LastChange  modules.ConsensusChangeID
	RenewHeight types.BlockHeight
	SpentPeriod types.Currency
	SpentTotal  types.Currency
}

// save saves the hostdb persistence data to disk.
func (c *Contractor) save() error {
	data := contractorPersist{
		Allowance:   c.allowance,
		LastChange:  c.lastChange,
		RenewHeight: c.renewHeight,
		SpentPeriod: c.spentPeriod,
		SpentTotal:  c.spentTotal,
	}
	for _, contract := range c.contracts {
		data.Contracts = append(data.Contracts, contract)
	}
	return c.persist.save(data)
}

// load loads the Contractor persistence data from disk.
func (c *Contractor) load() error {
	var data contractorPersist
	err := c.persist.load(&data)
	if err != nil {
		return err
	}
	c.allowance = data.Allowance
	for _, contract := range data.Contracts {
		c.contracts[contract.ID] = contract
	}
	c.lastChange = data.LastChange
	c.renewHeight = data.RenewHeight
	c.spentPeriod = data.SpentPeriod
	c.spentTotal = data.SpentTotal
	return nil
}
