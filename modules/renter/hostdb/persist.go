package hostdb

// hdbPersist defines what HostDB data persists across sessions.
type hdbPersist struct {
	// TODO: save hosts
}

// save saves the hostdb persistence data to disk.
func (hdb *HostDB) save() error {
	return nil
}

// load loads the hostdb persistence data from disk.
func (hdb *HostDB) load() error {
	return nil
}
