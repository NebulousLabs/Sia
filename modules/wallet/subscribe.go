package wallet

// notifySubscribers tells each subscriber that the wallet has received an
// update.
func (w *Wallet) notifySubscribers() {
	for _, subscriber := range w.subscribers {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

// WalletSubscribe adds a subscriber to the wallet.
func (w *Wallet) WalletSubscribe() <-chan struct{} {
	c := make(chan struct{}, 1)
	id := w.mu.Lock()
	w.subscribers = append(w.subscribers, c)
	w.mu.Unlock(id)
	return c
}
