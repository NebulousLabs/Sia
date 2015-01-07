package wallet

func (w *Wallet) lock() {
	w.rwLock.Lock()
}

func (w *Wallet) unlock() {
	w.rwLock.Unlock()
}

func (w *Wallet) rLock() {
	w.rwLock.RLock()
}

func (w *Wallet) rUnlock() {
	w.rwLock.RUnlock()
}
