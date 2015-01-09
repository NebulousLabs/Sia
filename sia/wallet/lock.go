package wallet

// lock write-locks the wallet.
func (w *Wallet) lock() {
	w.rwLock.Lock()
}

// unlock write-unlocks the wallet.
func (w *Wallet) unlock() {
	w.rwLock.Unlock()
}

// rLock read-locks the wallet.
func (w *Wallet) rLock() {
	w.rwLock.RLock()
}

// rUnlock read-unlocks the wallet.
func (w *Wallet) rUnlock() {
	w.rwLock.RUnlock()
}
