package contractor

// contractorPersist defines what Contractor data persists across sessions.
type contractorPersist struct {
	Contracts []hostContract
}

// save saves the hostdb persistence data to disk.
func (c *Contractor) save() error {
	var data contractorPersist
	for _, hc := range c.contracts {
		data.Contracts = append(data.Contracts, hc)
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
	for _, hc := range data.Contracts {
		c.contracts[hc.ID] = hc
	}
	return nil
}
