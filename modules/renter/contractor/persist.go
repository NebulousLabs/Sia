package contractor

import (
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// contractorPersist defines what Contractor data persists across sessions.
type contractorPersist struct {
	Allowance        modules.Allowance
	BlockHeight      types.BlockHeight
	CachedRevisions  []cachedRevision
	ContractMetrics  []modules.RenterContractMetrics
	Contracts        []modules.RenterContract
	FinancialMetrics modules.RenterFinancialMetrics
	LastChange       modules.ConsensusChangeID
	RenewedIDs       map[string]string
}

// persistData returns the data in the Contractor that will be saved to disk.
func (c *Contractor) persistData() contractorPersist {
	data := contractorPersist{
		Allowance:        c.allowance,
		BlockHeight:      c.blockHeight,
		FinancialMetrics: c.financialMetrics,
		LastChange:       c.lastChange,
		RenewedIDs:       make(map[string]string),
	}
	for _, rev := range c.cachedRevisions {
		data.CachedRevisions = append(data.CachedRevisions, rev)
	}
	for _, m := range c.contractMetrics {
		data.ContractMetrics = append(data.ContractMetrics, m)
	}
	for _, contract := range c.contracts {
		data.Contracts = append(data.Contracts, contract)
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
	for _, rev := range data.CachedRevisions {
		c.cachedRevisions[rev.revision.ParentID] = rev
	}
	for _, m := range data.ContractMetrics {
		c.contractMetrics[m.ID] = m
	}
	for _, contract := range data.Contracts {
		c.contracts[contract.ID] = contract
	}
	c.financialMetrics = data.FinancialMetrics
	c.lastChange = data.LastChange
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
	return c.persist.saveSync(c.persistData())
}

// saveRevision returns a function that saves a revision. It is used by the
// Editor and Downloader types to prevent desynchronizing with their host.
func (c *Contractor) saveRevision(id types.FileContractID) func(types.FileContractRevision, []crypto.Hash) error {
	return func(rev types.FileContractRevision, newRoots []crypto.Hash) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.cachedRevisions[id] = cachedRevision{rev, newRoots}
		return c.saveSync()
	}
}
