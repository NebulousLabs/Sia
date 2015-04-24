package renter

// updateSubscribers will inform all subscribers of the new update to the host.
func (r *Renter) updateSubscribers() {
	for _, subscriber := range r.subscriptions {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

// RenterNotify returns a channel that will be sent a struct{} every time there
// is an update received from another module.
func (r *Renter) RenterNotify() <-chan struct{} {
	lockID := r.mu.Lock()
	c := make(chan struct{}, 1)
	r.subscriptions = append(r.subscriptions, c)
	r.mu.Unlock(lockID)
	return c
}
