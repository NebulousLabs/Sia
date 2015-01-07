package hostdb

func (hdb *HostDB) Info() ([]byte, error) {
	return nil, nil
}

func (hdb *HostDB) Size() int {
	return len(hdb.activeHosts)
}
