package contractor

import (
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// contractorPersist defines what Contractor data persists across sessions.
type contractorPersist struct {
	Allowance       modules.Allowance
	BlockHeight     types.BlockHeight
	CachedRevisions []cachedRevision
	Contracts       []modules.RenterContract
	CurrentPeriod   types.BlockHeight
	LastChange      modules.ConsensusChangeID
	OldContracts    []modules.RenterContract
	RenewedIDs      map[string]string

	// COMPATv1.0.4-lts
	FinancialMetrics struct {
		ContractSpending types.Currency `json:"contractspending"`
		DownloadSpending types.Currency `json:"downloadspending"`
		StorageSpending  types.Currency `json:"storagespending"`
		UploadSpending   types.Currency `json:"uploadspending"`
	} `json:",omitempty"`
}

// persistData returns the data in the Contractor that will be saved to disk.
func (c *Contractor) persistData() contractorPersist {
	data := contractorPersist{
		Allowance:     c.allowance,
		BlockHeight:   c.blockHeight,
		CurrentPeriod: c.currentPeriod,
		LastChange:    c.lastChange,
		RenewedIDs:    make(map[string]string),
	}
	for _, rev := range c.cachedRevisions {
		data.CachedRevisions = append(data.CachedRevisions, rev)
	}
	for _, contract := range c.contracts {
		data.Contracts = append(data.Contracts, contract)
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
	for _, rev := range data.CachedRevisions {
		c.cachedRevisions[rev.Revision.ParentID] = rev
	}
	c.currentPeriod = data.CurrentPeriod
	if c.currentPeriod == 0 {
		// COMPATv1.0.4-lts
		// If loading old persist, current period will be unknown. Best we can
		// do is guess based on contracts + allowance.
		var highestEnd types.BlockHeight
		for _, contract := range data.Contracts {
			if h := contract.EndHeight(); h > highestEnd {
				highestEnd = h
			}
		}
		c.currentPeriod = highestEnd - c.allowance.Period
	}

	// Old perist may need information from the hostdb to fill in missing
	// fields.
	allHosts := c.hdb.AllHosts()
	hmap := make(map[modules.NetAddress]modules.HostDBEntry)
	// Iterate so that in the event of duplicate hosts for a netaddress, the
	// highest score host is the one in the map.
	for _, host := range allHosts {
		hmap[host.NetAddress] = host
	}

	for _, contract := range data.Contracts {
		// COMPATv1.0.4-lts
		// If loading old persist, start height of contract is unknown. Give
		// the contract a fake startheight so that it will included with the
		// other contracts in the current period.
		if contract.StartHeight == 0 {
			contract.StartHeight = c.currentPeriod + 1
		}

		// COMPATv1.1.0
		//
		// If loading old persist, the host's public key is unknown. Use the
		// hostdb to fill out the field.
		if len(contract.HostPublicKey.Key) == 0 {
			if entry, ok := hmap[contract.NetAddress]; ok {
				contract.HostPublicKey = entry.PublicKey
			}
		}

		c.contracts[contract.ID] = contract
	}
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

	// COMPATv1.0.4-lts
	//
	// If loading old persist, only aggregate metrics are known. Store these
	// in a special contract under a special identifier.
	if fm := data.FinancialMetrics; !fm.ContractSpending.Add(fm.DownloadSpending).Add(fm.StorageSpending).Add(fm.UploadSpending).IsZero() {
		c.oldContracts[metricsContractID] = modules.RenterContract{
			ID:               metricsContractID,
			TotalCost:        fm.ContractSpending,
			DownloadSpending: fm.DownloadSpending,
			StorageSpending:  fm.StorageSpending,
			UploadSpending:   fm.UploadSpending,
			// Give the contract a fake startheight so that it will included
			// with the other contracts in the current period. Note that in
			// update.go, the special contract is specifically deleted when a
			// new period begins.
			StartHeight: c.currentPeriod + 1,
			// We also need to add a ValidProofOutput so that the RenterFunds
			// method will not panic. The value should be 0, i.e. "all funds
			// were spent."
			LastRevision: types.FileContractRevision{
				NewValidProofOutputs: make([]types.SiacoinOutput, 2),
			},
		}
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
