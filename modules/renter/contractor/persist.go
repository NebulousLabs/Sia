package contractor

import (
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// contractorPersist defines what Contractor data persists across sessions.
type contractorPersist struct {
	Allowance         modules.Allowance          `json:"allowance"`
	BlockHeight       types.BlockHeight          `json:"blockheight"`
	ContractUtilities map[string]contractUtility `json:"contractUtilities"`
	CurrentPeriod     types.BlockHeight          `json:"currentperiod"`
	LastChange        modules.ConsensusChangeID  `json:"lastchange"`
	OldContracts      []modules.RenterContract   `json:"oldcontracts"`
	RenewedIDs        map[string]string          `json:"renewedids"`
}

// persistData returns the data in the Contractor that will be saved to disk.
func (c *Contractor) persistData() contractorPersist {
	data := contractorPersist{
		Allowance:         c.allowance,
		BlockHeight:       c.blockHeight,
		ContractUtilities: make(map[string]contractUtility),
		CurrentPeriod:     c.currentPeriod,
		LastChange:        c.lastChange,
		RenewedIDs:        make(map[string]string),
	}
	for id, u := range c.contractUtilities {
		data.ContractUtilities[id.String()] = u
	}
	for _, contract := range c.oldContracts {
		data.OldContracts = append(data.OldContracts, contract)
	}
	for oldID, newID := range c.renewedIDs {
		data.RenewedIDs[oldID.String()] = newID.String()
	}
	return data
}

// load loads the Contractor persistence data from disk.
func (c *Contractor) load() error {
	var data contractorPersist
	err := c.persist.load(&data)
	if err != nil {
		return err
	}
	c.allowance = data.Allowance
	c.blockHeight = data.BlockHeight
	c.currentPeriod = data.CurrentPeriod
	c.lastChange = data.LastChange
	for _, contract := range data.OldContracts {
		c.oldContracts[contract.ID] = contract
	}
	for oldString, newString := range data.RenewedIDs {
		var oldHash, newHash crypto.Hash
		oldHash.LoadString(oldString)
		newHash.LoadString(newString)
		c.renewedIDs[types.FileContractID(oldHash)] = types.FileContractID(newHash)
	}

	return nil
}

// save saves the Contractor persistence data to disk.
func (c *Contractor) save() error {
	return c.persist.save(c.persistData())
}

// saveSync saves the Contractor persistence data to disk and then syncs to disk.
func (c *Contractor) saveSync() error {
	return c.persist.save(c.persistData())
}
