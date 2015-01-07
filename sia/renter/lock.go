package renter

func (r *Renter) lock() {
	r.rwLock.Lock()
}

func (r *Renter) unlock() {
	r.rwLock.Unlock()
}

func (r *Renter) rLock() {
	r.rwLock.RLock()
}

func (r *Renter) rUnlock() {
	r.rwLock.RUnlock()
}
