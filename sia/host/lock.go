package host

// lock write-locks the host.
func (h *Host) lock() {
	h.rwLock.Lock()
}

// unlock write-unlocks the host.
func (h *Host) unlock() {
	h.rwLock.Unlock()
}

// rLock read-locks the host.
func (h *Host) rLock() {
	h.rwLock.RLock()
}

// rUnlock read-unlocks the host.
func (h *Host) rUnlock() {
	h.rwLock.RUnlock()
}
