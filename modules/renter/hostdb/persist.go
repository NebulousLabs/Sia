package hostdb

// hdbPersist defines what HostDB data persists across sessions.
type hdbPersist struct {
	Contracts []hostContract
}

// save saves the hostdb persistence data to disk.
func (hdb *HostDB) save() error {
	var data hdbPersist
	for _, hc := range hdb.contracts {
		data.Contracts = append(data.Contracts, hc)
	}
	return hdb.persist.save(data)
}

// load loads the hostdb persistence data from disk.
func (hdb *HostDB) load() error {
	var data hdbPersist
	err := hdb.persist.load(&data)
	if err != nil {
		return err
	}
	for _, hc := range data.Contracts {
		hdb.contracts[hc.ID] = hc
	}
	return nil
}
