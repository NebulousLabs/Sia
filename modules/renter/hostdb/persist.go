package hostdb

// hdbPersist defines what HostDB data persists across sessions.
type hdbPersist struct {
	AllHosts    []hostEntry
	ActiveHosts []hostEntry
}

// save saves the hostdb persistence data to disk.
func (hdb *HostDB) save() error {
	var data hdbPersist
	for _, entry := range hdb.allHosts {
		data.AllHosts = append(data.AllHosts, *entry)
	}
	for _, node := range hdb.activeHosts {
		data.ActiveHosts = append(data.ActiveHosts, *node.hostEntry)
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
	for _, entry := range data.AllHosts {
		hdb.allHosts[entry.NetAddress] = &entry
	}
	for _, entry := range data.ActiveHosts {
		hdb.insertNode(&entry)
	}
	return nil
}
