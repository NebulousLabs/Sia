package renter

// lock write-locks the renter.
func (r *Renter) lock() {
	r.rwLock.Lock()
}

// unlock write-unlocks the renter.
func (r *Renter) unlock() {
	r.rwLock.Unlock()
}

// rLock read-locks the renter.
func (r *Renter) rLock() {
	r.rwLock.RLock()
}

// rUnlock read-unlocks the renter.
func (r *Renter) rUnlock() {
	r.rwLock.RUnlock()
}
