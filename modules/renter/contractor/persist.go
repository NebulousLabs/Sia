package contractor

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// contractorPersist defines what Contractor data persists across sessions.
type contractorPersist struct {
	Allowance        modules.Allowance
	BlockHeight      types.BlockHeight
	CachedRevisions  []types.FileContractRevision
	Contracts        []modules.RenterContract
	LastChange       modules.ConsensusChangeID
	RenewHeight      types.BlockHeight
	FinancialMetrics modules.RenterFinancialMetrics
}

// persistData returns the data in the Contractor that will be saved to disk.
func (c *Contractor) persistData() contractorPersist {
	data := contractorPersist{
		Allowance:        c.allowance,
		BlockHeight:      c.blockHeight,
		LastChange:       c.lastChange,
		RenewHeight:      c.renewHeight,
		FinancialMetrics: c.financialMetrics,
	}
	for _, rev := range c.cachedRevisions {
		data.CachedRevisions = append(data.CachedRevisions, rev)
	}
	for _, contract := range c.contracts {
		data.Contracts = append(data.Contracts, contract)
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
	for _, rev := range data.CachedRevisions {
		c.cachedRevisions[rev.ParentID] = rev
	}
	for _, contract := range data.Contracts {
		c.contracts[contract.ID] = contract
	}
	c.lastChange = data.LastChange
	c.renewHeight = data.RenewHeight
	c.financialMetrics = data.FinancialMetrics
	return nil
}

// save saves the Contractor persistence data to disk.
func (c *Contractor) save() error {
	return c.persist.save(c.persistData())
}

// saveSync saves the Contractor persistence data to disk and then syncs to disk.
func (c *Contractor) saveSync() error {
	return c.persist.saveSync(c.persistData())
}

// saveRevision returns a function that saves a revision. It is used by the
// Editor and Downloader types to prevent desynchronizing with their host.
func (c *Contractor) saveRevision(id types.FileContractID) func(types.FileContractRevision) error {
	return func(rev types.FileContractRevision) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.cachedRevisions[id] = rev
		return c.saveSync()
	}
}
