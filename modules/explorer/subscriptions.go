package explorer

import (
	"github.com/NebulousLabs/Sia/modules"
)

// updateSubscribers will inform subscribers of any updates to the block explorer
func (e *Explorer) updateSubscribers() {
	e.updates++
	for _, subscriber := range e.subscriptions {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

// ExplorerNotify returns a channel that will be sent an empty
// struct every time there is an update recieved from from another module
func (e *Explorer) ExplorerNotify() <-chan struct{} {
	c := make(chan struct{}, modules.NotifyBuffer)
	lockID := e.mu.Lock()
	for i := uint64(0); i < e.updates; i++ {
		c <- struct{}{}
	}
	e.subscriptions = append(e.subscriptions, c)
	e.mu.Unlock(lockID)
	return c
}
