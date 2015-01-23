package hostdb

func (hdb *HostDB) Info() ([]byte, error) {
	return nil, nil
}

func (hdb *HostDB) Size() int {
	hdb.mu.RLock()
	defer hdb.mu.RUnlock()
	return len(hdb.activeHosts)
}
