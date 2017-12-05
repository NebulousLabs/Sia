package contractor

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/proto"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

// contractorPersist defines what Contractor data persists across sessions.
type contractorPersist struct {
	Allowance         modules.Allowance          `json:"allowance"`
	BlockHeight       types.BlockHeight          `json:"blockheight"`
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
		CurrentPeriod:     c.currentPeriod,
		LastChange:        c.lastChange,
		RenewedIDs:        make(map[string]string),
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
		if p, ok := c.persist.(*stdPersist); ok {
			// try loading old persist
			err = c.loadv130Contracts(p.filename)
		}
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

// loadv130Contracts converts the old contract journal format to the new
// per-contract file format.
func (c *Contractor) loadv130Contracts(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	// decode the journal metadata
	dec := json.NewDecoder(f)
	var meta persist.Metadata
	if err = dec.Decode(&meta); err != nil {
		return err
	} else if meta.Header != "Contractor Journal" {
		return fmt.Errorf("expected header %q, got %q", "Contractor Journal", meta.Header)
	} else if meta.Version != "1.1.1" {
		return fmt.Errorf("journal version (%s) is incompatible with the current version (%s)", meta.Version, "1.1.1")
	}

	// decode the old journal checkpoint
	var checkpoint struct {
		Contracts map[string]proto.V130Contract `json:"contracts"`
	}
	if err = dec.Decode(&checkpoint); err != nil {
		return err
	}
	for _, contract := range checkpoint.Contracts {
		if err := c.contracts.ImportV130Contract(contract); err != nil {
			return err
		}
	}
	return nil
}
