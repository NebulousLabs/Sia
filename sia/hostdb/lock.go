package hostdb

func (hdb *HostDB) lock() {
	hdb.rwLock.Lock()
}

func (hdb *HostDB) unlock() {
	hdb.rwLock.Unlock()
}

func (hdb *HostDB) rLock() {
	hdb.rwLock.RLock()
}

func (hdb *HostDB) rUnlock() {
	hdb.rwLock.RUnlock()
}
