package hostdb

// lock.go is in place merely as a convenience. Instead of needing to write
// hdb.lock.Lock(), you just write hdb.lock(), and we don't need to export the
// functions.

func (hdb *HostDB) lock() {
	hdb.dbLock.Lock()
}

func (hdb *HostDB) unlock() {
	hdb.dbLock.Unlock()
}

func (hdb *HostDB) rLock() {
	hdb.dbLock.RLock()
}

func (hdb *HostDB) rUnlock() {
	hdb.dbLock.RUnlock()
}
