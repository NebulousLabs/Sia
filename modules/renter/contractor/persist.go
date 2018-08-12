package contractor

import (
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/proto"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/errors"
)

// contractorPersist defines what Contractor data persists across sessions.
type contractorPersist struct {
	Allowance     modules.Allowance         `json:"allowance"`
	BlockHeight   types.BlockHeight         `json:"blockheight"`
	CurrentPeriod types.BlockHeight         `json:"currentperiod"`
	LastChange    modules.ConsensusChangeID `json:"lastchange"`
	OldContracts  []modules.RenterContract  `json:"oldcontracts"`
}

// persistData returns the data in the Contractor that will be saved to disk.
func (c *Contractor) persistData() contractorPersist {
	data := contractorPersist{
		Allowance:     c.allowance,
		BlockHeight:   c.blockHeight,
		CurrentPeriod: c.currentPeriod,
		LastChange:    c.lastChange,
	}
	for _, contract := range c.oldContracts {
		data.OldContracts = append(data.OldContracts, contract)
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

// convertPersist converts the pre-v1.3.1 contractor persist formats to the new
// formats.
func convertPersist(dir string) error {
	// Try loading v1.3.1 persist. If it has the correct version number, no
	// further action is necessary.
	persistPath := filepath.Join(dir, "contractor.json")
	err := persist.LoadJSON(persistMeta, nil, persistPath)
	if err == nil {
		return nil
	}

	// Try loading v1.3.0 persist (journal).
	journalPath := filepath.Join(dir, "contractor.journal")
	if _, err := os.Stat(journalPath); os.IsNotExist(err) {
		// no journal file found; assume this is a fresh install
		return nil
	}
	var p journalPersist
	j, err := openJournal(journalPath, &p)
	if err != nil {
		return err
	}
	j.Close()
	// convert to v1.3.1 format and save
	data := contractorPersist{
		Allowance:     p.Allowance,
		BlockHeight:   p.BlockHeight,
		CurrentPeriod: p.CurrentPeriod,
		LastChange:    p.LastChange,
	}
	for _, c := range p.OldContracts {
		data.OldContracts = append(data.OldContracts, modules.RenterContract{
			ID:               c.ID,
			HostPublicKey:    c.HostPublicKey,
			StartHeight:      c.StartHeight,
			EndHeight:        c.EndHeight(),
			RenterFunds:      c.RenterFunds(),
			DownloadSpending: c.DownloadSpending,
			StorageSpending:  c.StorageSpending,
			UploadSpending:   c.UploadSpending,
			TotalCost:        c.TotalCost,
			ContractFee:      c.ContractFee,
			TxnFee:           c.TxnFee,
			SiafundFee:       c.SiafundFee,
		})
	}
	err = persist.SaveJSON(persistMeta, data, persistPath)
	if err != nil {
		return err
	}

	// create the contracts directory if it does not yet exist
	cs, err := proto.NewContractSet(filepath.Join(dir, "contracts"), modules.ProdDependencies)
	if err != nil {
		return err
	}
	defer cs.Close()

	// convert contracts to contract files
	for _, c := range p.Contracts {
		cachedRev := p.CachedRevisions[c.ID.String()]
		if err := cs.ConvertV130Contract(c, cachedRev); err != nil {
			return err
		}
	}

	// delete the journal file
	return errors.AddContext(os.Remove(journalPath), "failed to remove journal file")
}
