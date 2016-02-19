package contractor

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// contractorPersist defines what Contractor data persists across sessions.
type contractorPersist struct {
	Allowance   modules.Allowance
	Contracts   []Contract
	RenewHeight types.BlockHeight
}

// save saves the hostdb persistence data to disk.
func (c *Contractor) save() error {
	var data contractorPersist
	data.Allowance = c.allowance
	for _, contract := range c.contracts {
		data.Contracts = append(data.Contracts, contract)
	}
	data.RenewHeight = c.renewHeight
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
	c.renewHeight = data.RenewHeight
	return nil
}
