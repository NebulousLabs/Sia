package hostdb

func (hdb *HostDB) Info() ([]byte, error) {
	return nil, nil
}

func (hdb *HostDB) Size() int {
	hdb.rLock()
	defer hdb.rUnlock()
	return len(hdb.activeHosts)
}
