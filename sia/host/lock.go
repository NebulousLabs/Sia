package host

func (h *Host) lock() {
	h.rwLock.Lock()
}

func (h *Host) unlock() {
	h.rwLock.Unlock()
}

func (h *Host) rLock() {
	h.rwLock.RLock()
}

func (h *Host) rUnlock() {
	h.rwLock.RUnlock()
}
