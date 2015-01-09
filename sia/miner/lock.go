package miner

// lock write-locks the miner.
func (m *Miner) lock() {
	m.rwLock.Lock()
}

// unlock write-unlocks the miner.
func (m *Miner) unlock() {
	m.rwLock.Unlock()
}

// rLock read-locks the miner.
func (m *Miner) rLock() {
	m.rwLock.RLock()
}

// rUnlock read-unlocks the miner.
func (m *Miner) rUnlock() {
	m.rwLock.RUnlock()
}
