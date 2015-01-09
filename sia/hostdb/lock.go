package hostdb

// lock write-locks the hostDB.
func (hdb *HostDB) lock() {
	hdb.rwLock.Lock()
}

// unlock write-unlocks the hostDB.
func (hdb *HostDB) unlock() {
	hdb.rwLock.Unlock()
}

// rLock read-locks the hostDB.
func (hdb *HostDB) rLock() {
	hdb.rwLock.RLock()
}

// rUnlock read-unlocks the hostDB.
func (hdb *HostDB) rUnlock() {
	hdb.rwLock.RUnlock()
}
