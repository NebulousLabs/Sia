package renter

import (
	"github.com/NebulousLabs/Sia/modules"
)

// subscriptions.go is the standard module subscription file. Only Notify is
// implemented for the renter.

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
	c := make(chan struct{}, modules.NotifyBuffer)
	lockID := r.mu.Lock()
	if r.blockHeight > 0 {
		c <- struct{}{}
	}
	r.subscriptions = append(r.subscriptions, c)
	r.mu.Unlock(lockID)
	return c
}
