package miner

func (m *Miner) lock() {
	m.rwLock.Lock()
}

func (m *Miner) unlock() {
	m.rwLock.Unlock()
}

func (m *Miner) rLock() {
	m.rwLock.RLock()
}

func (m *Miner) rUnlock() {
	m.rwLock.RUnlock()
}
